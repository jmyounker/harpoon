package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"syscall"
)

// container is a high level interface to an operating system container. The
// API interacts directly with this interface.
type container interface {
	Create() error
	Instance() agent.ContainerInstance
	Destroy() error
	Start() error
	Stop() error
	Subscribe(ch chan<- agent.ContainerInstance)
	Unsubscribe(ch chan<- agent.ContainerInstance)
	Logs() *containerLog
	Recover() error
	Exit()
}

const (
	maxContainerIDLength       = 256 // TODO(pb): enforce this limit at creation-time
	containerLogRingBufferSize = 10000
)

type realContainer struct {
	agent.ContainerInstance

	containerRoot string
	portRange     *portRange
	logs          *containerLog

	supervisor      *supervisor
	containerStatec chan agent.ContainerProcessState

	subscribers map[chan<- agent.ContainerInstance]struct{}

	actionc chan actionRequest
	subc    chan chan<- agent.ContainerInstance
	unsubc  chan chan<- agent.ContainerInstance

	quitc chan chan struct{}
}

// Satisfaction guaranteed.
var _ container = &realContainer{}

func newContainer(id string, containerRoot string, config agent.ContainerConfig, pr *portRange) *realContainer {
	c := &realContainer{
		ContainerInstance: agent.ContainerInstance{
			ID:              id,
			ContainerStatus: agent.ContainerStatusCreated,
			ContainerConfig: config,
		},

		containerRoot: containerRoot,
		portRange:     pr,
		logs:          newContainerLog(containerLogRingBufferSize),

		subscribers: map[chan<- agent.ContainerInstance]struct{}{},

		actionc:         make(chan actionRequest),
		subc:            make(chan chan<- agent.ContainerInstance),
		unsubc:          make(chan chan<- agent.ContainerInstance),
		containerStatec: make(chan agent.ContainerProcessState),
		quitc:           make(chan chan struct{}),
	}

	go c.loop()

	return c
}

func (c *realContainer) Create() error {
	req := actionRequest{
		action: containerCreate,
		res:    make(chan error),
	}
	c.actionc <- req
	return <-req.res
}

func (c *realContainer) Destroy() error {
	req := actionRequest{
		action: containerDestroy,
		res:    make(chan error),
	}
	c.actionc <- req
	return <-req.res
}

func (c *realContainer) Logs() *containerLog {
	return c.logs
}

func (c *realContainer) Instance() agent.ContainerInstance {
	return c.ContainerInstance
}

func (c *realContainer) Start() error {
	req := actionRequest{
		action: containerStart,
		res:    make(chan error),
	}
	c.actionc <- req
	return <-req.res
}

func (c *realContainer) Stop() error {
	req := actionRequest{
		action: containerStop,
		res:    make(chan error),
	}
	c.actionc <- req
	return <-req.res
}

func (c *realContainer) Subscribe(ch chan<- agent.ContainerInstance) {
	c.subc <- ch
}

func (c *realContainer) Unsubscribe(ch chan<- agent.ContainerInstance) {
	c.unsubc <- ch
}

func (c *realContainer) loop() {
	defer c.logs.exit()

	for {
		select {
		case req := <-c.actionc:
			// All of these methods must be nonblocking
			switch req.action {
			case containerCreate:
				incContainerCreate(1)
				err := c.create()
				if err != nil {
					incContainerCreateFailure(1)
				}
				req.res <- err

			case containerDestroy:
				incContainerDestroy(1)
				err := c.destroy()
				req.res <- err
				if err == nil {
					return
				}

			case containerStart:
				incContainerStart(1)
				err := c.start()
				if err != nil {
					incContainerStartFailure(1)
				}
				req.res <- err

			case containerStop:
				incContainerStop(1)
				req.res <- c.stop()

			default:
				panic(fmt.Sprintf("unknown action %q", req.action))
			}

		case ch := <-c.subc:
			c.subscribers[ch] = struct{}{}

		case state := <-c.containerStatec:
			if state.Up {
				c.updateStatus(agent.ContainerStatusRunning)
			}

			if state.Up || state.Restarting {
				continue
			}

			// The container is down and will not be restarted; begin teardown and
			// update state.
			c.supervisor.Unsubscribe(c.containerStatec)

			if state.Err != "" {
				log.Printf("[%s] failed: %s", c.ID, state.Err)
				c.updateStatus(agent.ContainerStatusFailed)
				continue
			}

			c.updateStatus(agent.ContainerStatusFinished)

			c.supervisor.Exit()

		case ch := <-c.unsubc:
			delete(c.subscribers, ch)

		case quitc := <-c.quitc:
			close(quitc)
			return
		}
	}
}

// Recover attempts to recover an existing container which may or may not be running.
func (c *realContainer) Recover() error {
	var (
		rundir = filepath.Join(c.containerRoot, c.ID)
		logdir = filepath.Join("/srv/harpoon/log/", c.ID)
	)

	if err := c.validateConfig(); err != nil {
		return err
	}

	for _, port := range c.ContainerConfig.Ports {
		c.portRange.claimPort(port)
	}

	logPipe, err := startLogger(c.ID, logdir)
	if err != nil {
		return err
	}
	// ensure we don't hold on to the logger
	defer logPipe.Close()

	c.supervisor = newSupervisor(c.ID, rundir)

	_, err = os.Stat(filepath.Join(rundir, "control"))
	if err == syscall.ENOENT || err == syscall.ENOTDIR {
		return err
	}
	if err == nil {
		exitedc := make(chan error, 1)
		c.supervisor.attach(exitedc)
		c.supervisor.Subscribe(c.containerStatec)
	}

	return nil
}

func (c *realContainer) Exit() {
	q := make(chan struct{})
	c.quitc <- q
	<-q
}

func (c *realContainer) create() error {
	var (
		rundir = filepath.Join(c.containerRoot, c.ID)
		logdir = filepath.Join("/srv/harpoon/log/", c.ID)

		agentJSONPath     = filepath.Join(rundir, "agent.json")
		rootfsSymlinkPath = filepath.Join(rundir, "rootfs")
		logSymlinkPath    = filepath.Join(rundir, "log")
	)

	if err := c.validateConfig(); err != nil {
		return err
	}

	err := c.assignPorts()
	if err != nil {
		return fmt.Errorf("could not assign ports: %s", err)
	}

	// expand variables in command
	command := c.ContainerConfig.Command.Exec
	for i, arg := range command {
		command[i] = os.Expand(arg, func(k string) string {
			return c.ContainerConfig.Env[k]
		})
	}

	// Create directories need for container
	if err := os.MkdirAll(rundir, 0775); err != nil {
		return fmt.Errorf("mkdir all %s: %s", rundir, err)
	}

	if err := os.MkdirAll(logdir, 0775); err != nil {
		return fmt.Errorf("mkdir all %s: %s", logdir, err)
	}

	// Write agent config file
	agentFile, err := os.OpenFile(agentJSONPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer agentFile.Close()

	if err := json.NewEncoder(agentFile).Encode(c.ContainerConfig); err != nil {
		return err
	}

	if *debug {
		log.Printf("agent file written to: %s", agentJSONPath)
	}
	// TODO(pb): it's a problem that this is blocking
	rootfs, err := c.fetchArtifact()
	if err != nil {
		return fmt.Errorf("fetch: %s", err)
	}

	if err := os.Symlink(rootfs, rootfsSymlinkPath); err != nil && !os.IsExist(err) {
		return fmt.Errorf("symlink rootfs: %s", err)
	}

	if err := os.Symlink(logdir, logSymlinkPath); err != nil && !os.IsExist(err) {
		return fmt.Errorf("symlink log: %s", err)
	}

	if *debug {
		log.Printf("artifact successfully retrieved and unpacked")
	}
	return nil
}

func (c *realContainer) validateConfig() error {
	if c.ContainerConfig.Env == nil {
		c.ContainerConfig.Env = map[string]string{}
	}

	for _, source := range c.ContainerConfig.Storage.Volumes {
		if _, ok := configuredVolumes[source]; !ok {
			return fmt.Errorf("container depends on missing volume %q", source)
		}
	}

	for dest, size := range c.ContainerConfig.Storage.Temp {
		if size != -1 {
			return fmt.Errorf("cannot make tmpfs %q %dMB: sized tmpfs storage not yet supported", dest, size)
		}
	}

	return nil
}

// assignPorts assigns any automatic ports, updating the config's port and
// environment maps.
func (c *realContainer) assignPorts() error {
	for name, port := range c.ContainerConfig.Ports {
		if port == 0 {
			var err error
			port, err = c.portRange.getPort()
			if err != nil {
				return err
			}
		}

		portName := fmt.Sprintf("PORT_%s", strings.ToUpper(name))

		c.ContainerConfig.Ports[name] = port
		c.ContainerConfig.Env[portName] = strconv.Itoa(int(port))
	}
	return nil
}

func (c *realContainer) destroy() error {
	var (
		rundir = filepath.Join(c.containerRoot, c.ID)
	)

	switch c.ContainerInstance.ContainerStatus {
	default:
	case agent.ContainerStatusRunning:
		return fmt.Errorf("can't destroy container in status %s", c.ContainerInstance.ContainerStatus)
	}

	c.updateStatus(agent.ContainerStatusDeleted)

	// Return assigned ports
	for _, port := range c.ContainerConfig.Ports {
		c.portRange.returnPort(port) // doesn't matter if we try to remove unallocated ports
	}

	err := os.RemoveAll(rundir)
	if err != nil {
		return err
	}

	for subc := range c.subscribers {
		close(subc)
	}

	c.subscribers = map[chan<- agent.ContainerInstance]struct{}{}

	return nil
}

func (c *realContainer) fetchArtifact() (string, error) {
	var (
		artifactURL                            = c.ContainerConfig.ArtifactURL
		artifactPath, artifactCompression, err = getArtifactDetails(artifactURL)
	)

	if err != nil {
		return "", err
	}

	log.Printf("fetching url %s to %s", artifactURL, artifactPath)

	if _, err := os.Stat(artifactPath); err == nil {
		return artifactPath, nil
	}

	if err := os.MkdirAll(artifactPath, 0755); err != nil {
		return "", err
	}

	resp, err := http.Get(artifactURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if err := extractArtifact(resp.Body, artifactPath, artifactCompression); err != nil {
		return "", err
	}

	return artifactPath, nil
}

func (c *realContainer) start() error {
	switch c.ContainerInstance.ContainerStatus {
	default:
		return fmt.Errorf("can't start container with status %s", c.ContainerInstance.ContainerStatus)
	case agent.ContainerStatusCreated, agent.ContainerStatusFinished, agent.ContainerStatusFailed:
	}

	var (
		rundir = path.Join(c.containerRoot, c.ID)
		logdir = filepath.Join("/srv/harpoon/log/", c.ID)
	)

	supervisorLog, err := os.Create(path.Join(rundir, "supervisor.log"))
	if err != nil {
		return fmt.Errorf("unable to create log file for supervisor: %s", err)
	}

	// don't hold on to this log file after exec or error
	defer supervisorLog.Close()

	logPipe, err := startLogger(c.ID, logdir)
	if err != nil {
		return err
	}

	// ensure we don't hold on to the logger
	defer logPipe.Close()

	s := newSupervisor(c.ID, rundir)

	if err := s.Start(c.ContainerConfig, logPipe, supervisorLog); err != nil {
		return err
	}

	s.Subscribe(c.containerStatec)
	c.supervisor = s

	return nil
}

func (c *realContainer) stop() error {
	switch c.ContainerInstance.ContainerStatus {
	default:
		return fmt.Errorf("can't stop container with status %s", c.ContainerInstance.ContainerStatus)
	case agent.ContainerStatusRunning:
	}

	c.supervisor.Stop(c.ContainerConfig.Grace.Shutdown.Duration)

	return nil
}

func (c *realContainer) updateStatus(status agent.ContainerStatus) {
	c.ContainerInstance.ContainerStatus = status

	for subc := range c.subscribers {
		subc <- c.ContainerInstance
	}
}

type containerAction string

const (
	containerCreate  containerAction = "create"
	containerDestroy                 = "destroy"
	containerStart                   = "start"
	containerStop                    = "stop"
)

type actionRequest struct {
	action containerAction
	res    chan error
}

func extractArtifact(src io.Reader, dst string, compression string) (err error) {
	defer func() {
		if err != nil {
			os.RemoveAll(dst)
		}
	}()

	cmd := exec.Command("tar", "-C", dst, "-x"+compression)
	cmd.Stdin = src

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("tar extraction failed: %s", err)
	}

	return nil
}

func getArtifactDetails(artifactURL string) (string, string, error) {
	parsed, err := url.Parse(artifactURL)
	if err != nil {
		return "", "", fmt.Errorf("unable to parse url: %s", err)
	}

	path := func(suffix string) string {
		return filepath.Join(
			"/srv/harpoon/artifacts",
			parsed.Host,
			strings.TrimSuffix(parsed.Path, suffix),
		)
	}

	switch true {
	case strings.HasSuffix(parsed.Path, ".tar"):
		return path(".tar"), "", nil
	case strings.HasSuffix(parsed.Path, ".tar.gz"):
		return path(".tar.gz"), "z", nil
	case strings.HasSuffix(parsed.Path, ".tgz"):
		return path(".tgz"), "z", nil
	case strings.HasSuffix(parsed.Path, ".tar.bz2"):
		return path(".tar.bz2"), "j", nil
	default:
		return "", "", fmt.Errorf("unknown suffix for artifact url: %s", artifactURL)
	}
}
