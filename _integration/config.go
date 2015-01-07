// generates a libcontainer config for integration tests
package main

import (
	"encoding/json"
	"flag"
	"log"
	"os"

	"github.com/docker/libcontainer"
	"github.com/docker/libcontainer/cgroups"
	"github.com/docker/libcontainer/devices"
	"github.com/docker/libcontainer/mount"
)

func main() {
	var rootfs = flag.String("rootfs", "", "rootfs")

	flag.Parse()

	if *rootfs == "" {
		log.Fatal("rootfs flag not provided")
	}

	config := libcontainer.Config{
		RootFs: *rootfs,

		WorkingDir: "/",

		Capabilities: []string{
			"CHOWN",
			"DAC_OVERRIDE",
			// FOWNER is necessary for tar to set file utimes.
			"FOWNER",
			// KILL is necessary for stopping containers.
			"KILL",
			"MKNOD",
			"SETGID",
			"SETUID",
			"SYS_ADMIN",
			"SYS_CHROOT",
		},

		Namespaces: map[string]bool{
			"NEWNS":  true, // mounts
			"NEWUTS": true, // hostname
			"NEWIPC": true, // system V ipc
			"NEWPID": true, // pid
		},

		Env: []string{
			"PATH=/bin:/srv/harpoon/bin",
		},

		Cgroups: &cgroups.Cgroup{
			Name:           "agent",
			Parent:         "harpoon-integration",
			AllowedDevices: devices.DefaultAllowedDevices,
		},

		MountConfig: &libcontainer.MountConfig{
			DeviceNodes: devices.DefaultAllowedDevices,
			Mounts: []*mount.Mount{
				{Type: "bind", Source: "/etc/resolv.conf", Destination: "/etc/resolv.conf", Private: true},
				{Type: "bind", Source: "/sys/fs/cgroup", Destination: "/sys/fs/cgroup", Writable: true, Private: true},
			},
		},
	}

	buf, _ := json.MarshalIndent(config, "", "  ")
	buf = append(buf, '\n')

	os.Stdout.Write(buf)
}
