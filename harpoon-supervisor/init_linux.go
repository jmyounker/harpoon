// +build linux

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"runtime"
	"syscall"

	"github.com/docker/libcontainer"
	"github.com/docker/libcontainer/namespaces"
	"github.com/docker/libcontainer/syncpipe"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

const DefaultFDs uint64 = 4069

func init() {
	// If the process name is harpoon-container-init (set by commandBuilder in
	// container.go), execution will be hijacked from main().
	if os.Args[0] != containerInitName {
		return
	}

	// Locking the thread here ensures that we stay in the main thread, which in
	// turn ensures that our parent death signal hasn't been reset.
	runtime.LockOSThread()

	// If the sync pipe cannot be initialized, there's no way to report an error
	// except by logging it and exiting nonzero. Once the sync pipe is set up
	// errors are communicated over it instead of through logging.
	syncPipe, err := syncpipe.NewSyncPipeFromFd(0, uintptr(3))
	if err != nil {
		fmt.Fprintf(os.Stderr, "unable to initialize sync pipe: %s", err)
		os.Exit(2)
	}

	// redirect stderr to stdout; harpoon-container will not write to stderr from
	// this point on.
	syscall.Dup2(syscall.Stdout, syscall.Stderr)

	defer func() {
		if e := recover(); e != nil {
			syncPipe.ReportChildError(fmt.Errorf("panic: %s", e))
			os.Exit(2)
		}
	}()

	args := os.Args[1:]

	if len(args) == 0 {
		syncPipe.ReportChildError(fmt.Errorf("no command given for container"))
		os.Exit(2)
	}

	// Load and parse Harpoon agent Config
	agentFile, err := os.Open(agentFileName)
	if err != nil {
		syncPipe.ReportChildError(fmt.Errorf("unable to open %q: %s", agentFileName, err))
		os.Exit(2)
	}
	defer agentFile.Close()

	var agentConfig agent.ContainerConfig
	if err := json.NewDecoder(agentFile).Decode(&agentConfig); err != nil {
		syncPipe.ReportChildError(fmt.Errorf("unable to parse %q: %s", agentFileName, err))
		os.Exit(2)
	}

	// Set file descriptor count.
	fds := agentConfig.Resources.FD
	if fds == 0 {
		fds = DefaultFDs
	}
	err = syscall.Setrlimit(syscall.RLIMIT_NOFILE, &syscall.Rlimit{Cur: fds, Max: fds})
	if err != nil {
		syncPipe.ReportChildError(fmt.Errorf("could not set fd limits to %d: %s", fds, err))
		os.Exit(2)
	}

	f, err := os.Open(containerFileName)
	if err != nil {
		syncPipe.ReportChildError(fmt.Errorf("unable to open %q: %s", containerFileName, err))
		os.Exit(2)
	}

	var container *libcontainer.Config

	if err := json.NewDecoder(f).Decode(&container); err != nil {
		syncPipe.ReportChildError(fmt.Errorf("unable to parse %q: %s", containerFileName, err))
		os.Exit(2)
	}

	namespaces.Init(container, rootfsFileName, "", syncPipe, args)

	// If we get past namespaces.Init(), that means the container failed to exec.
	// The error will have already been reported to the called via syncPipe, so
	// we simply exit nonzero.
	os.Exit(2)
}
