// +build linux

package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"syscall"

	"github.com/docker/libcontainer"
	"github.com/docker/libcontainer/cgroups"
	"github.com/docker/libcontainer/cgroups/fs"
	"github.com/docker/libcontainer/devices"
	"github.com/docker/libcontainer/mount"
	"github.com/docker/libcontainer/namespaces"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type container struct {
	hostname string
	id       string

	agentConfigPath     string
	agentConfig         agent.ContainerConfig
	containerConfigPath string
	rootfs              string
	args                []string

	err             error
	containerConfig *libcontainer.Config

	cmd   *exec.Cmd
	oomc  <-chan struct{}
	exitc chan error
}

func newContainer(hostname string, id string, agentConfig, containerConfig, rootfs string, args []string) Container {
	container := &container{
		hostname:            hostname,
		id:                  id,
		agentConfigPath:     agentConfig,
		containerConfigPath: containerConfig,
		rootfs:              rootfs,
		args:                args,
		exitc:               make(chan error, 1),
	}

	container.err = container.configure()
	return container
}

// configure validates and loads the container's configuration
func (c *container) configure() error {
	if len(c.args) == 0 {
		return fmt.Errorf("no command given for container")
	}

	// Load and parse Harpoon agent Config
	agentConfigFile, err := os.Open(c.agentConfigPath)
	if err != nil {
		return err
	}
	defer agentConfigFile.Close()

	if err := json.NewDecoder(agentConfigFile).Decode(&c.agentConfig); err != nil {
		return err
	}

	// Check if the rootfs exists
	fi, err := os.Stat(c.rootfs)
	if err != nil {
		return fmt.Errorf("unable to stat rootfs %q: %s", c.rootfs, err)
	}

	if !fi.IsDir() {
		return fmt.Errorf("rootfs %q invalid: not a directory", c.rootfs)
	}

	// Extract libcontainer config from harpoon config, and write it out to the filesystem.
	c.containerConfig = c.libcontainerConfig()
	containerConfigFile, err := os.OpenFile(c.containerConfigPath, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0644)
	if err != nil {
		return err
	}
	defer containerConfigFile.Close()

	buf, err := json.MarshalIndent(c.containerConfig, "", "  ")
	if err != nil {
		return fmt.Errorf("unable to marshal container config: %s", err)
	}

	if _, err := containerConfigFile.Write(buf); err != nil {
		return fmt.Errorf("Could not write container config: %s", err)
	}

	return nil
}

// containerCommand implements namespaces.CreateCommand.
func (c *container) containerCommand(container *libcontainer.Config, _, _, init string, childPipe *os.File, args []string) *exec.Cmd {
	cmd := exec.Command(init, args...)
	cmd.Args[0] = containerInitName
	cmd.ExtraFiles = []*os.File{childPipe}

	cmd.SysProcAttr = &syscall.SysProcAttr{
		Cloneflags: uintptr(namespaces.GetNamespaceFlags(container.Namespaces)),
		Pdeathsig:  syscall.SIGKILL,
	}

	c.cmd = cmd
	return cmd
}

func (c *container) Start() error {
	if c.err != nil {
		return c.err
	}

	var started = make(chan struct{})

	startCallback := func() {
		oom, err := fs.NotifyOnOOM(c.containerConfig.Cgroups)

		if err != nil {
			log.Print("unable to set up oom notifications: ", err)
		}

		c.oomc = oom
		started <- struct{}{}
	}

	go func() {
		_, err := namespaces.Exec(
			c.containerConfig,
			os.Stdin,
			os.Stdout,
			os.Stderr,
			"", // no console
			"", // datapath handled elsewhere
			c.args,
			c.containerCommand,
			startCallback,
		)

		c.exitc <- err
	}()

	select {
	case err := <-c.exitc:
		return err
	case <-started:
	}

	return nil
}

// wait blocks until the container process exits.
func (c *container) wait() (oomed bool) {
	for {
		select {
		case <-c.exitc:
			return oomed

		case _, ok := <-c.oomc:
			if !ok {
				continue
			}

			oomed = true
		}
	}
}

func (c *container) Wait() agent.ContainerExitStatus {
	var (
		oomed = c.wait()
		ws    = c.cmd.ProcessState.Sys().(syscall.WaitStatus)
	)

	switch {
	case oomed && ws.Signaled() && ws.Signal() == syscall.SIGKILL:
		// The linux oomkiller kills with SIGKILL, so if we receive an OOM
		// notification and the container exits from SIGKILL, report exit status as
		// OOMed.
		return agent.ContainerExitStatus{
			OOMed: true,
		}
	case ws.Exited():
		return agent.ContainerExitStatus{
			Exited:     true,
			ExitStatus: ws.ExitStatus(),
		}
	case ws.Signaled():
		return agent.ContainerExitStatus{
			Signaled: true,
			Signal:   int(ws.Signal()),
		}
	}

	return agent.ContainerExitStatus{}
}

func (c *container) Signal(sig os.Signal) {
	if c.cmd == nil || c.cmd.Process == nil {
		return
	}

	c.cmd.Process.Signal(sig)
}

func (c *container) Metrics() agent.ContainerMetrics {
	stats, err := fs.GetStats(c.containerConfig.Cgroups)
	if err != nil {
		return agent.ContainerMetrics{}
	}

	return agent.ContainerMetrics{
		MemoryUsage: stats.MemoryStats.Usage,
		MemoryLimit: stats.MemoryStats.Stats["hierarchical_memory_limit"],
		CPUTime:     stats.CpuStats.CpuUsage.TotalUsage,
	}
}

// libcontainerConfig builds a complete libcontainer.Config from an
// agent.ContainerConfig.
func (c *container) libcontainerConfig() *libcontainer.Config {
	var (
		config = &libcontainer.Config{
			Hostname: c.hostname,
			// daemon user and group; must be numeric as we make no assumptions about
			// the presence or contents of "/etc/passwd" in the container.
			User:       "1:1",
			WorkingDir: c.agentConfig.Command.WorkingDir,
			Namespaces: map[string]bool{
				"NEWNS":  true, // mounts
				"NEWUTS": true, // hostname
				"NEWIPC": true, // system V ipc
				"NEWPID": true, // pid
			},
			Cgroups: &cgroups.Cgroup{
				Name:   c.id,
				Parent: "harpoon",

				Memory: int64(c.agentConfig.Resources.Memory * 1024 * 1024),

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

	for k, v := range c.agentConfig.Env {
		config.Env = append(config.Env, fmt.Sprintf("%s=%s", k, v))
	}

	for dest, source := range c.agentConfig.Storage.Volumes {
		config.MountConfig.Mounts = append(config.MountConfig.Mounts, &mount.Mount{
			Type: "bind", Source: source, Destination: dest, Writable: true, Private: true,
		})
	}

	for dest := range c.agentConfig.Storage.Temp {
		config.MountConfig.Mounts = append(config.MountConfig.Mounts, &mount.Mount{
			Type: "tmpfs", Destination: dest, Writable: true, Private: true,
		})
	}

	return config
}
