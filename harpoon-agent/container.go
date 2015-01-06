package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

// container is a high level interface to an operating system container. The
// API interacts directly with this interface. Implementers of this interface
// must be safe for concurrent access.
type container interface {
	Create(unregisterAtFailure func(), downloadTimeout time.Duration) error
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

	debug             bool
	configuredVolumes volumes
	containerRoot     string
	portDB            *portDB
	logs              *containerLog
	supervisor        *supervisor
	containerStatec   chan agent.ContainerProcessState
	subscribers       map[chan<- agent.ContainerInstance]struct{}
	createc           chan createRequest
	destroyc          chan destroyRequest
	startc            chan startRequest
	stopc             chan stopRequest
	subc              chan chan<- agent.ContainerInstance
	unsubc            chan chan<- agent.ContainerInstance
	quitc             chan chan struct{}
}

// Satisfaction guaranteed.
var _ container = &realContainer{}

func newRealContainer(
	id string,
	containerRoot string,
	configuredVolumes volumes,
	config agent.ContainerConfig,
	debug bool,
	pdb *portDB,
) container {
	c := &realContainer{
		ContainerInstance: agent.ContainerInstance{
			ID:              id,
			ContainerStatus: agent.ContainerStatusCreated,
			ContainerConfig: config,
		},

		configuredVolumes: configuredVolumes,
		containerRoot:     containerRoot,
		debug:             debug,
		portDB:            pdb,
		logs:              newContainerLog(containerLogRingBufferSize),
		subscribers:       map[chan<- agent.ContainerInstance]struct{}{},
		createc:           make(chan createRequest),
		destroyc:          make(chan destroyRequest),
		startc:            make(chan startRequest),
		stopc:             make(chan stopRequest),
		subc:              make(chan chan<- agent.ContainerInstance),
		unsubc:            make(chan chan<- agent.ContainerInstance),
		containerStatec:   make(chan agent.ContainerProcessState),
		quitc:             make(chan chan struct{}),
	}

	go c.loop()

	return c
}

func (c *realContainer) Create(unregister func(), downloadTimeout time.Duration) error {
	req := createRequest{
		unregister:      unregister,
		downloadTimeout: downloadTimeout,
		resp:            make(chan error),
	}
	c.createc <- req
	return <-req.resp
}

func (c *realContainer) Destroy() error {
	req := destroyRequest{
		resp: make(chan error),
	}
	c.destroyc <- req
	return <-req.resp
}

func (c *realContainer) Logs() *containerLog {
	return c.logs
}

func (c *realContainer) Instance() agent.ContainerInstance {
	return c.ContainerInstance
}

func (c *realContainer) Start() error {
	req := startRequest{
		resp: make(chan error),
	}
	c.startc <- req
	return <-req.resp
}

func (c *realContainer) Stop() error {
	req := stopRequest{
		resp: make(chan error),
	}
	c.stopc <- req
	return <-req.resp
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
		// All methods here must be nonblocking.
		select {
		case req := <-c.createc:
			incContainerCreate(1)
			err := c.create(req.unregister, req.downloadTimeout)
			if err != nil {
				incContainerCreateFailure(1)
			}
			req.resp <- err

		case req := <-c.destroyc:
			incContainerDestroy(1)
			err := c.destroy()
			req.resp <- err
			if err == nil {
				return
			}

		case req := <-c.startc:
			incContainerStart(1)
			err := c.start()
			if err != nil {
				incContainerStartFailure(1)
			}
			req.resp <- err

		case req := <-c.stopc:
			incContainerStop(1)
			req.resp <- c.stop()

		case ch := <-c.subc:
			c.subscribers[ch] = struct{}{}

		case state := <-c.containerStatec:
			c.ContainerInstance.ContainerProcessState = state
			if state.Up {
				c.updateStatus(agent.ContainerStatusRunning)
				continue
			}

			if state.Restarting {
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

	err := c.portDB.claimPorts(c.ContainerConfig.Ports)
	if err != nil {
		return err
	}

	logPipe, err := startLogger(c.ID, logdir)
	if err != nil {
		return err
	}
	// ensure we don't hold on to the logger
	defer logPipe.Close()

	c.supervisor = newSupervisor(c.ID, rundir, c.debug)

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

func (c *realContainer) create(unregister func(), downloadTimeout time.Duration) error {
	var (
		rundir = filepath.Join(c.containerRoot, c.ID)
		logdir = filepath.Join("/srv/harpoon/log/", c.ID)

		agentJSONPath = filepath.Join(rundir, "agent.json")
	)

	if err := c.validateConfig(); err != nil {
		return err
	}

	success := false
	defer func() {
		if !success {
			c.destroy()
			unregister()
		}
	}()

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

	if c.debug {
		log.Printf("agent file written to: %s", agentJSONPath)
	}

	go c.secondPhaseCreate(unregister, downloadTimeout)

	success = true
	return nil
}

func (c *realContainer) secondPhaseCreate(unregister func(), downloadTimeout time.Duration) {
	var (
		rundir = filepath.Join(c.containerRoot, c.ID)
		logdir = filepath.Join("/srv/harpoon/log/", c.ID)

		rootfsSymlinkPath = filepath.Join(rundir, "rootfs")
		logSymlinkPath    = filepath.Join(rundir, "log")
	)

	success := false
	defer func() {
		if !success {
			c.destroy()
			unregister()
		}
	}()

	rootfs, err := c.fetchArtifact(downloadTimeout)
	if err != nil {
		log.Printf("fetch: %s", err)
		return
	}

	if err := os.Symlink(rootfs, rootfsSymlinkPath); err != nil && !os.IsExist(err) {
		log.Printf("symlink rootfs: %s", err)
		return
	}

	if err := os.Symlink(logdir, logSymlinkPath); err != nil && !os.IsExist(err) {
		log.Printf("symlink log: %s", err)
		return
	}

	if c.debug {
		log.Printf("artifact successfully retrieved and unpacked")
	}

	success = true
	c.updateStatus(agent.ContainerStatusCreated)
}

func (c *realContainer) validateConfig() error {
	if c.ContainerConfig.Env == nil {
		c.ContainerConfig.Env = map[string]string{}
	}

	for _, source := range c.ContainerConfig.Storage.Volumes {
		if _, ok := c.configuredVolumes[source]; !ok {
			return fmt.Errorf("container depends on missing volume %q", source)
		}
	}

	for dest, size := range c.ContainerConfig.Storage.Tmp {
		if size != -1 {
			return fmt.Errorf("cannot make tmpfs %q %dMB: sized tmpfs storage not yet supported", dest, size)
		}
	}

	return nil
}

// assignPorts assigns any automatic ports, updating the config's port and
// environment maps.
func (c *realContainer) assignPorts() error {
	if err := c.portDB.acquirePorts(c.ContainerConfig.Ports); err != nil {
		return err
	}
	for name, port := range c.ContainerConfig.Ports {
		portName := fmt.Sprintf("PORT_%s", strings.ToUpper(name))
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

	c.portDB.releasePorts(c.ContainerConfig.Ports)

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

func (c *realContainer) fetchArtifact(downloadTimeout time.Duration) (string, error) {
	var (
		artifactURL                            = c.ContainerConfig.ArtifactURL
		artifactPath, artifactCompression, err = getArtifactDetails(artifactURL)
	)

	if err != nil {
		return "", err
	}

	log.Printf("fetching URL %s to %s", artifactURL, artifactPath)

	if _, err := os.Stat(artifactPath); err == nil {
		return artifactPath, nil
	}

	if err := os.MkdirAll(artifactPath, 0755); err != nil {
		return "", err
	}

	client := http.Client{Timeout: downloadTimeout}
	resp, err := client.Get(artifactURL)
	if err != nil {
		incContainerArtifactDownloadFailure(1)
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
	case agent.ContainerStatusCreated, agent.ContainerStatusFinished, agent.ContainerStatusFailed:
	default:
		return fmt.Errorf("can't start container with status %s", c.ContainerInstance.ContainerStatus)
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

	s := newSupervisor(c.ID, rundir, c.debug)

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

func extractArtifact(src io.Reader, dst string, compression string) (err error) {
	defer func() {
		if err != nil {
			os.RemoveAll(dst)
		}
	}()

	log.Printf("extracting with cmd: tar -xv%s -C %s ", compression, dst)
	cmd := exec.Command("tar", "-xv"+compression, "-C", dst)
	cmd.Stdin = src

	// It's incredibly hard to debug failures in tar unless stderr is available.
	//
	// Sadly tar has error conditions in which it reports success even though it fails.
	// (Seen when missing the FOWNER capability.)  Therefore we can't depend upon the
	// error code to determine when to report stderr.
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return errors.New("could not open tar's stderr")
	}

	if err := cmd.Start(); err != nil {
		log.Printf("could not start tar command: %s", err)
	}

	tarout, err := ioutil.ReadAll(stderr)
	if err != nil {
		log.Printf("error while reading from tar's stderr: %s", err)
	}
	if len(tarout) != 0 {
		log.Printf("stderr from tar: %s", string(tarout))
	}

	if err := cmd.Wait(); err != nil {
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

type createRequest struct {
	unregister      func()
	downloadTimeout time.Duration
	resp            chan error
}

type destroyRequest struct {
	resp chan error
}

type startRequest struct {
	resp chan error
}

type stopRequest struct {
	resp chan error
}
