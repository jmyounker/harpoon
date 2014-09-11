package main

import (
	"encoding/json"
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
	"time"

	"github.com/docker/libcontainer"
	"github.com/docker/libcontainer/cgroups"
	"github.com/docker/libcontainer/devices"
	"github.com/docker/libcontainer/mount"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

// container is a high level interface to an operating system container. The
// API interacts directly with this interface.
type container interface {
	Create() error
	Instance() agent.ContainerInstance
	Destroy() error
	Heartbeat(hb agent.Heartbeat) string
	Start() error
	Stop() error
	Subscribe(ch chan<- agent.ContainerInstance)
	Unsubscribe(ch chan<- agent.ContainerInstance)
	Logs() *containerLog
}

const (
	maxContainerIDLength       = 256 // TODO(pb): enforce this limit at creation-time
	containerLogRingBufferSize = 10000
)

type realContainer struct {
	agent.ContainerInstance

	desired      string
	downDeadline time.Time
	logs         *containerLog

	subscribers map[chan<- agent.ContainerInstance]struct{}

	actionc    chan actionRequest
	heartbeatc chan heartbeatRequest
	subc       chan chan<- agent.ContainerInstance
	unsubc     chan chan<- agent.ContainerInstance
}

// Satisfaction guaranteed.
var _ container = &realContainer{}

func newContainer(id string, config agent.ContainerConfig) *realContainer {
	c := &realContainer{
		ContainerInstance: agent.ContainerInstance{
			ID:              id,
			ContainerStatus: agent.ContainerStatusCreated,
			ContainerConfig: config,
		},

		logs: newContainerLog(containerLogRingBufferSize),

		subscribers: map[chan<- agent.ContainerInstance]struct{}{},

		actionc:    make(chan actionRequest),
		heartbeatc: make(chan heartbeatRequest),
		subc:       make(chan chan<- agent.ContainerInstance),
		unsubc:     make(chan chan<- agent.ContainerInstance),
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

func (c *realContainer) Heartbeat(hb agent.Heartbeat) string {
	req := heartbeatRequest{
		heartbeat: hb,
		res:       make(chan string),
	}
	c.heartbeatc <- req
	return <-req.res
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

		case req := <-c.heartbeatc:
			req.res <- c.heartbeat(req.heartbeat)

		case ch := <-c.subc:
			c.subscribers[ch] = struct{}{}

		case ch := <-c.unsubc:
			delete(c.subscribers, ch)
		}
	}
}

func (c *realContainer) create() error {
	var (
		rundir = filepath.Join("/run/harpoon", c.ID)
		logdir = filepath.Join("/srv/harpoon/log/", c.ID)

		containerJSONPath = filepath.Join(rundir, "container.json")
		rootfsSymlinkPath = filepath.Join(rundir, "rootfs")
		logSymlinkPath    = filepath.Join(rundir, "log")
	)

	if err := c.validateConfig(); err != nil {
		return err
	}

	c.assignPorts()

	// expand variables in command
	command := c.ContainerConfig.Command.Exec
	for i, arg := range command {
		command[i] = os.Expand(arg, func(k string) string {
			return c.ContainerConfig.Env[k]
		})
	}

	config, err := json.Marshal(c.libcontainerConfig())
	if err != nil {
		return err
	}

	if err := os.MkdirAll(rundir, os.ModePerm); err != nil {
		return fmt.Errorf("mkdir all %s: %s", rundir, err)
	}

	if err := os.MkdirAll(logdir, os.ModePerm); err != nil {
		return fmt.Errorf("mkdir all %s: %s", logdir, err)
	}

	if err := ioutil.WriteFile(containerJSONPath, config, os.ModePerm); err != nil {
		return err
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

	return nil
}

func (c *realContainer) validateConfig() error {
	if c.ContainerConfig.Env == nil {
		c.ContainerConfig.Env = map[string]string{}
	}

	for _, source := range c.ContainerConfig.Storage.Volumes {
		if _, ok := configuredVolumes[source]; !ok {
			return fmt.Errorf("container depends on missing volume %q")
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
func (c *realContainer) assignPorts() {
	for name, port := range c.ContainerConfig.Ports {
		if port == 0 {
			port = uint16(nextPort())
		}

		portName := fmt.Sprintf("PORT_%s", strings.ToUpper(name))

		c.ContainerConfig.Ports[name] = port
		c.ContainerConfig.Env[portName] = strconv.Itoa(int(port))
	}
}

// libcontainerConfig builds a complete libcontainer.Config from an
// agent.ContainerConfig.
func (c *realContainer) libcontainerConfig() *libcontainer.Config {
	var (
		config = &libcontainer.Config{
			Hostname: hostname,
			// daemon user and group; must be numeric as we make no assumptions about
			// the presence or contents of "/etc/passwd" in the container.
			User:       "1:1",
			WorkingDir: c.ContainerConfig.Command.WorkingDir,
			Namespaces: map[string]bool{
				"NEWNS":  true, // mounts
				"NEWUTS": true, // hostname
				"NEWIPC": true, // system V ipc
				"NEWPID": true, // pid
			},
			Cgroups: &cgroups.Cgroup{
				Name:   c.ID,
				Parent: "harpoon",

				Memory: int64(c.ContainerConfig.Resources.Memory * 1024 * 1024),

				AllowedDevices: devices.DefaultAllowedDevices,
			},
			MountConfig: &libcontainer.MountConfig{
				DeviceNodes: devices.DefaultAllowedDevices,
				Mounts: []*mount.Mount{
					{Type: "bind", Source: "/etc/resolv.conf", Destination: "/etc/resolv.conf", Private: true},
				},
				ReadonlyFs: true,
			},
		}
	)

	for k, v := range c.ContainerConfig.Env {
		config.Env = append(config.Env, fmt.Sprintf("%s=%s", k, v))
	}

	for dest, source := range c.ContainerConfig.Storage.Volumes {
		config.MountConfig.Mounts = append(config.MountConfig.Mounts, &mount.Mount{
			Type: "bind", Source: source, Destination: dest, Writable: true, Private: true,
		})
	}

	for dest := range c.ContainerConfig.Storage.Temp {
		config.MountConfig.Mounts = append(config.MountConfig.Mounts, &mount.Mount{
			Type: "tmpfs", Destination: dest, Writable: true, Private: true,
		})
	}

	return config
}

func (c *realContainer) destroy() error {
	var (
		rundir = filepath.Join("/run/harpoon", c.ID)
	)

	switch c.ContainerInstance.ContainerStatus {
	default:
	case agent.ContainerStatusRunning:
		return fmt.Errorf("can't destroy container in status %s", c.ContainerInstance.ContainerStatus)
	}

	c.updateStatus(agent.ContainerStatusDeleted)

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

// heartbeat takes the Heartbeat from the container process (the actual
// state), compares with our desired state, and returns a string that'll be
// packaged and sent in the HeartbeatReply, to tell the container process what
// to do next.
//
// This also potentially updates the status, but it can only possibly move to
// ContainerStatusFinished.
func (c *realContainer) heartbeat(hb agent.Heartbeat) string {
	switch want, have := c.desired, hb.Status; true {
	case want == "UP" && have == "UP":
		// Normal state, running
		return "UP"

	case want == "UP" && have == "DOWN":
		// Container stopped, for whatever reason
		c.updateStatus(agent.ContainerStatusFinished)
		return "DOWN" // TODO(pb): should it be a third state?

	case want == "DOWN" && have == "UP":
		// Waiting for the container to shutdown normally
		if time.Now().After(c.downDeadline) {
			incContainerStatusKilled(1)
			return "FORCEDOWN" // too long: kill -9
		}
		return "DOWN" // keep waiting

	case want == "DOWN" && have == "DOWN":
		// Normal shutdown successful; won't receive more updates
		incContainerStatusDownSuccessful(1)
		c.updateStatus(agent.ContainerStatusFinished)
		return "DOWN" // TODO(pb): this was FORCEDOWN, but DOWN makes more sense to me?

	case want == "FORCEDOWN" && have == "UP":
		// Waiting for the container to shutdown aggressively
		return "FORCEDOWN"

	case want == "FORCEDOWN" && have == "DOWN":
		// Aggressive shutdown successful
		incContainerStatusForceDownSuccessful(1)
		c.updateStatus(agent.ContainerStatusFinished)
		return "FORCEDOWN"
	}
	return "UNKNOWN"
}

func (c *realContainer) start() error {
	switch c.ContainerInstance.ContainerStatus {
	default:
		return fmt.Errorf("can't start container with status %s", c.ContainerInstance.ContainerStatus)
	case agent.ContainerStatusCreated, agent.ContainerStatusFinished, agent.ContainerStatusFailed:
	}

	var (
		rundir = path.Join("/run/harpoon", c.ID)
		logdir = filepath.Join("/srv/harpoon/log/", c.ID)
	)

	containerLog, err := os.Create(path.Join(rundir, "container.log"))
	if err != nil {
		return fmt.Errorf("unable to create log file for container: %s", err)
	}

	// don't hold on to this log file after exec or error
	defer containerLog.Close()

	logPipe, err := startLogger(c.ID, logdir)
	if err != nil {
		return err
	}

	// ensure we don't hold on to the logger
	defer logPipe.Close()

	cmd := exec.Command(
		"harpoon-container",
		c.ContainerConfig.Command.Exec...,
	)

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf(
		"heartbeat_url=http://%s/api/v0/containers/%s/heartbeat",
		*addr,
		c.ID,
	))

	cmd.Stdout = logPipe
	cmd.Stderr = containerLog
	cmd.Dir = rundir

	c.desired = "UP"

	if err := cmd.Start(); err != nil {
		// update state
		return err
	}

	// no zombies
	go cmd.Wait()

	// reflect state
	c.updateStatus(agent.ContainerStatusRunning) // TODO(pb): intermediate Starting state?

	// TODO(pb): utilize Startup grace period somehow?

	return nil
}

func (c *realContainer) stop() error {
	switch c.ContainerInstance.ContainerStatus {
	default:
		return fmt.Errorf("can't stop container with status %s", c.ContainerInstance.ContainerStatus)
	case agent.ContainerStatusRunning:
	}

	c.desired = "DOWN"
	c.downDeadline = time.Now().Add(c.ContainerConfig.Grace.Shutdown.Duration).Add(heartbeatInterval)

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

type heartbeatRequest struct {
	heartbeat agent.Heartbeat
	res       chan string
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
		return err
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

// HACK
var port = make(chan int)

func init() {
	go func() {
		i := 30000

		for {
			port <- i
			i++
		}
	}()
}

func nextPort() int {
	return <-port
}
