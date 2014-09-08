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
	"github.com/docker/libcontainer/cgroups/fs"
	"github.com/docker/libcontainer/namespaces"
)

type container struct {
	config string
	rootfs string
	args   []string

	err             error
	containerConfig *libcontainer.Config

	cmd   *exec.Cmd
	oomc  <-chan struct{}
	exitc chan error
}

func newContainer(config, rootfs string, args []string) Container {
	container := &container{
		config: config,
		rootfs: rootfs,
		args:   args,
		exitc:  make(chan error, 1),
	}

	container.err = container.configure()
	return container
}

// configure validates and loads the container's configuration
func (c *container) configure() error {
	if len(c.args) == 0 {
		return fmt.Errorf("no command given for container")
	}

	fi, err := os.Stat(c.rootfs)
	if err != nil {
		return fmt.Errorf("unable to stat rootfs %q: %s", c.rootfs, err)
	}

	if !fi.IsDir() {
		return fmt.Errorf("rootfs %q invalid: not a directory", c.rootfs)
	}

	f, err := os.Open(c.config)
	if err != nil {
		return fmt.Errorf("unable to open %s: %s", c.config, err)
	}
	defer f.Close()

	var containerConfig libcontainer.Config

	if err := json.NewDecoder(f).Decode(&containerConfig); err != nil {
		return fmt.Errorf("unable to decode %s: %s", c.config, err)
	}

	c.containerConfig = &containerConfig
	return nil
}

// containerCommand implements namespaces.CreateCommand.
func (c *container) containerCommand(container *libcontainer.Config, _, _, _, init string, childPipe *os.File, args []string) *exec.Cmd {
	cmd := exec.Command(init, args...)
	cmd.Args[0] = "harpoon-container-init"
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
			"",     // no console
			"", "", // rootfs and datapath handled elsewhere
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

func (c *container) Wait() ContainerExitStatus {
	var oomed bool

wait:
	for {
		select {
		case <-c.exitc:
			break wait

		case _, ok := <-c.oomc:
			if !ok {
				continue
			}

			// TODO(bernerd): a boolean is likely insufficient here, since the
			// container may continue to run even after an OOM notification.
			oomed = true
		}
	}

	ws := c.cmd.ProcessState.Sys().(syscall.WaitStatus)

	switch {
	case oomed:
		return ContainerExitStatus{
			OOMed: true,
		}
	case ws.Exited():
		return ContainerExitStatus{
			Exited:     true,
			ExitStatus: ws.ExitStatus(),
		}
	case ws.Signaled():
		return ContainerExitStatus{
			Signaled: true,
			Signal:   int(ws.Signal()),
		}
	}

	return ContainerExitStatus{}
}

func (c *container) Signal(sig os.Signal) {
	if c.cmd == nil || c.cmd.Process == nil {
		return
	}

	c.cmd.Process.Signal(sig)
}

func (c *container) Metrics() ContainerMetrics {
	stats, err := fs.GetStats(c.containerConfig.Cgroups)
	if err != nil {
		return ContainerMetrics{}
	}

	return ContainerMetrics{
		MemoryUsage: stats.MemoryStats.Usage,
		MemoryLimit: stats.MemoryStats.Stats["hierarchical_memory_limit"],
		CPUTime:     stats.CpuStats.CpuUsage.TotalUsage,
	}
}
