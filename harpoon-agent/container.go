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

const maxContainerIDLength = 256 // TODO(pb): enforce this limit at creation-time

type container struct {
	agent.ContainerInstance

	config       *libcontainer.Config
	desired      string
	downDeadline time.Time
	logs         *containerLog

	subscribers map[chan<- agent.ContainerInstance]struct{}

	actionc    chan actionRequest
	heartbeatc chan heartbeatRequest
	subc       chan chan<- agent.ContainerInstance
	unsubc     chan chan<- agent.ContainerInstance
	quitc      chan struct{}
}

func newContainer(id string, config agent.ContainerConfig) *container {
	c := &container{
		ContainerInstance: agent.ContainerInstance{
			ID:     id,
			Status: agent.ContainerStatusCreated,
			Config: config,
		},
		logs:        NewContainerLog(10000),
		subscribers: map[chan<- agent.ContainerInstance]struct{}{},
		actionc:     make(chan actionRequest),
		heartbeatc:  make(chan heartbeatRequest),
		subc:        make(chan chan<- agent.ContainerInstance),
		unsubc:      make(chan chan<- agent.ContainerInstance),
		quitc:       make(chan struct{}),
	}

	c.buildContainerConfig()

	go c.loop()

	return c
}

func (c *container) Create() error {
	req := actionRequest{
		action: containerCreate,
		res:    make(chan error),
	}
	c.actionc <- req
	return <-req.res
}

func (c *container) Destroy() error {
	req := actionRequest{
		action: containerDestroy,
		res:    make(chan error),
	}
	c.actionc <- req
	return <-req.res
}

func (c *container) Heartbeat(hb agent.Heartbeat) string {
	req := heartbeatRequest{
		heartbeat: hb,
		res:       make(chan string),
	}
	c.heartbeatc <- req
	return <-req.res
}

func (c *container) Instance() agent.ContainerInstance {
	return c.ContainerInstance
}

func (c *container) Start() error {
	req := actionRequest{
		action: containerStart,
		res:    make(chan error),
	}
	c.actionc <- req
	return <-req.res
}

func (c *container) Stop() error {
	req := actionRequest{
		action: containerStop,
		res:    make(chan error),
	}
	c.actionc <- req
	return <-req.res
}

func (c *container) Subscribe(ch chan<- agent.ContainerInstance) {
	c.subc <- ch
}

func (c *container) Unsubscribe(ch chan<- agent.ContainerInstance) {
	c.unsubc <- ch
}

func (c *container) loop() {
	for {
		select {
		case req := <-c.actionc:
			// All of these methods must be nonblocking
			switch req.action {
			case containerCreate:
				req.res <- c.create()
			case containerDestroy:
				req.res <- c.destroy()
			case containerStart:
				req.res <- c.start()
			case containerStop:
				req.res <- c.stop()
			default:
				panic("unknown action")
			}
		case req := <-c.heartbeatc:
			req.res <- c.heartbeat(req.heartbeat)
		case ch := <-c.subc:
			c.subscribers[ch] = struct{}{}
		case ch := <-c.unsubc:
			delete(c.subscribers, ch)
		case <-c.quitc:
			c.logs.Exit()
			return
		}
	}
}

func (c *container) buildContainerConfig() {
	var (
		env    = []string{}
		mounts = mount.Mounts{
			{Type: "devtmpfs"},
			{Type: "bind", Source: "/etc/resolv.conf", Destination: "/etc/resolv.conf", Private: true},
		}
	)

	if c.Config.Env == nil {
		c.Config.Env = map[string]string{}
	}

	for k, v := range c.Config.Env {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	for dest, source := range c.Config.Storage.Volumes {
		if _, ok := configuredVolumes[source]; !ok {
			// TODO: this needs to happen as a part of a validation step, so the
			// container is rejected.
			log.Printf("volume %s not configured", source)
			continue
		}

		mounts = append(mounts, mount.Mount{
			Type: "bind", Source: source, Destination: dest, Private: true,
		})
	}

	c.config = &libcontainer.Config{
		Hostname: hostname,
		// daemon user and group; must be numeric as we make no assumptions about
		// the presence or contents of "/etc/passwd" in the container.
		User:       "1:1",
		WorkingDir: c.Config.Command.WorkingDir,
		Env:        env,
		Namespaces: map[string]bool{
			"NEWNS":  true, // mounts
			"NEWUTS": true, // hostname
			"NEWIPC": true, // uh...
			"NEWPID": true, // pid
		},
		Cgroups: &cgroups.Cgroup{
			Name:   c.ID,
			Parent: "harpoon",

			Memory: int64(c.Config.Resources.Memory * 1024 * 1024),

			AllowedDevices: devices.DefaultAllowedDevices,
		},
		MountConfig: &libcontainer.MountConfig{
			DeviceNodes: devices.DefaultAllowedDevices,
			Mounts:      mounts,
			ReadonlyFs:  true,
		},
	}
}

func (c *container) create() error {
	var (
		rundir = filepath.Join("/run/harpoon", c.ID)
		logdir = filepath.Join("/srv/harpoon/log/", c.ID)
	)

	if err := os.MkdirAll(rundir, os.ModePerm); err != nil {
		return fmt.Errorf("mkdir all %s: %s", rundir, err)
	}

	if err := os.MkdirAll(logdir, os.ModePerm); err != nil {
		return fmt.Errorf("mkdir all %s: %s", logdir, err)
	}

	// TODO(pb): it's a problem that this is blocking
	rootfs, err := c.fetchArtifact()
	if err != nil {
		return err
	}

	if err := os.Symlink(rootfs, filepath.Join(rundir, "rootfs")); err != nil && !os.IsExist(err) {
		return err
	}

	if err := os.Symlink(logdir, filepath.Join(rundir, "log")); err != nil && !os.IsExist(err) {
		return err
	}

	for name, port := range c.Config.Ports {
		if port == 0 {
			port = uint16(nextPort())
		}

		portName := fmt.Sprintf("PORT_%s", strings.ToUpper(name))

		c.Config.Ports[name] = port
		c.Config.Env[portName] = strconv.Itoa(int(port))
	}

	// expand variable in command
	command := c.Config.Command.Exec
	for i, arg := range command {
		command[i] = os.Expand(arg, func(k string) string {
			return c.Config.Env[k]
		})
	}

	return c.writeContainerJSON(filepath.Join(rundir, "container.json"))
}

func (c *container) destroy() error {
	var (
		rundir = filepath.Join("/run/harpoon", c.ID)
	)

	switch c.ContainerInstance.Status {
	default:
	case agent.ContainerStatusCreated, agent.ContainerStatusRunning:
		return fmt.Errorf("can't destroy container in status %s", c.ContainerInstance.Status)
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
	close(c.quitc)

	return nil
}

func (c *container) fetchArtifact() (string, error) {
	var (
		artifactURL  = c.Config.ArtifactURL
		artifactPath = getArtifactPath(artifactURL)
	)

	fmt.Fprintf(os.Stderr, "fetching url %s to %s\n", artifactURL, artifactPath)

	if !strings.HasSuffix(artifactURL, ".tar.gz") {
		return "", fmt.Errorf("artifact must be .tar.gz")
	}

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

	if err := extractArtifact(resp.Body, artifactPath); err != nil {
		return "", err
	}

	return artifactPath, nil
}

// heartbeat takes the Heartbeat from the container process (the actual
// state), compares with our desired state, and returns a string that'll be
// packaged and sent in the HeartbeatReply, to tell the container process what
// to do next.
//
// This also potentially updates the ContainerInstance.Status, but it can only
// possibly move to ContainerStatusFinished.
func (c *container) heartbeat(hb agent.Heartbeat) string {
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
			return "FORCEDOWN" // too long: kill -9
		}
		return "DOWN" // keep waiting

	case want == "DOWN" && have == "DOWN":
		// Normal shutdown successful; won't receive more updates
		c.updateStatus(agent.ContainerStatusFinished)
		return "DOWN" // TODO(pb): this was FORCEDOWN, but DOWN makes more sense to me?

	case want == "FORCEDOWN" && have == "UP":
		// Waiting for the container to shutdown aggressively
		return "FORCEDOWN"

	case want == "FORCEDOWN" && have == "DOWN":
		// Aggressive shutdown successful
		c.updateStatus(agent.ContainerStatusFinished)
		return "FORCEDOWN"
	}
	return "UNKNOWN"
}

func (c *container) start() error {
	switch c.ContainerInstance.Status {
	default:
		return fmt.Errorf("can't start container with status %s", c.ContainerInstance.Status)
	case agent.ContainerStatusCreated, agent.ContainerStatusFinished, agent.ContainerStatusFailed:
	}

	var (
		rundir = path.Join("/run/harpoon", c.ID)
		logdir = filepath.Join("/srv/harpoon/log/", c.ID)
	)

	logPipe, err := startLogger(c.ID, logdir)
	if err != nil {
		return err
	}

	// ensure we don't hold on to the logger
	defer logPipe.Close()

	cmd := exec.Command(
		"harpoon-container",
		c.Config.Command.Exec...,
	)

	cmd.Env = os.Environ()
	cmd.Env = append(cmd.Env, fmt.Sprintf(
		"heartbeat_url=http://%s/containers/%s/heartbeat",
		*addr,
		c.ID,
	))

	cmd.Stdout = logPipe
	cmd.Stderr = logPipe
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

func (c *container) stop() error {
	switch c.ContainerInstance.Status {
	default:
		return fmt.Errorf("can't stop container with status %s", c.ContainerInstance.Status)
	case agent.ContainerStatusRunning:
	}

	c.desired = "DOWN"
	c.downDeadline = time.Now().Add(time.Duration(c.Config.Grace.Shutdown) * time.Second).Add(heartbeatInterval)

	return nil
}

func (c *container) updateStatus(status agent.ContainerStatus) {
	c.ContainerInstance.Status = status

	for subc := range c.subscribers {
		subc <- c.ContainerInstance
	}
}

func (c *container) writeContainerJSON(dst string) error {
	data, err := json.Marshal(c.config)
	if err != nil {
		return err
	}

	return ioutil.WriteFile(dst, data, os.ModePerm)
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

func extractArtifact(src io.Reader, dst string) (err error) {
	defer func() {
		if err != nil {
			os.RemoveAll(dst)
		}
	}()

	cmd := exec.Command("tar", "-C", dst, "-zx")
	cmd.Stdin = src

	if err := cmd.Run(); err != nil {
		return err
	}

	return nil
}

func getArtifactPath(artifactURL string) string {
	parsed, err := url.Parse(artifactURL)
	if err != nil {
		panic(fmt.Sprintf("unable to parse url: %s", err))
	}

	return filepath.Join(
		"/srv/harpoon/artifacts",
		parsed.Host,
		strings.TrimSuffix(parsed.Path, ".tar.gz"),
	)
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
