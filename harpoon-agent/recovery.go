package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

// recoverContainers restores container states from disk, e.g., after
// harpoon-agent is restarted.
func recoverContainers(containerRoot string, r *registry, pdb *portDB) {
	// Get only containers which have been successfully started
	containerFilePaths, err := filepath.Glob(filepath.Join(containerRoot, "*", "container.json"))
	if err != nil {
		log.Println("unable to scan rundir for container files: ", err)
		return
	}

	// We got nothin!
	if len(containerFilePaths) == 0 {
		return
	}

	// Attempt to restore all the containers
	for _, containerFilePath := range containerFilePaths {
		incContainerRecoveryAttempts(1)
		containerDir := filepath.Dir(containerFilePath)
		containerRoot := filepath.Dir(containerDir)
		id := filepath.Base(containerDir)

		err := recoverContainer(id, containerRoot, r, pdb)
		if err == nil {
			log.Printf("recovered container %q from %s", id, containerDir)
			continue
		}
		log.Printf("failed to recover container %q from %s: %s", id, containerDir, err)
	}
}

func recoverContainer(id string, containerRoot string, r *registry, pdb *portDB) error {
	agentFilePath := filepath.Join(containerRoot, id, "agent.json")
	agentFile, err := os.Open(agentFilePath)
	if err != nil {
		return fmt.Errorf("could not read agent file: %s", err)
	}
	defer agentFile.Close()

	var agentConfig agent.ContainerConfig
	if err := json.NewDecoder(agentFile).Decode(&agentConfig); err != nil {
		return fmt.Errorf("could not parse agent file: %s", err)
	}

	c := newContainer(id, containerRoot, agentConfig, pdb)
	if err := c.Recover(); err != nil {
		c.Exit()
		return err
	}

	r.register(c)

	return nil
}
