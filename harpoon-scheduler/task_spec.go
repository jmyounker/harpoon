package main

import (
	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type taskSpec struct {
	Endpoint              string `json:"endpoint"`
	JobName               string `json:"job_name"`
	TaskName              string `json:"task_name"`
	ContainerID           string `json:"container_id"`
	agent.ContainerConfig `json:"container_config"`
}
