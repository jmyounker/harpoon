package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"strings"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func writeServiceDiscovery(filename string, instances []agent.ContainerInstance) (err error) {
	f, err := ioutil.TempFile("", "harpoon-agent-service-discovery")
	if err != nil {
		return err
	}

	defer func() {
		if err != nil {
			os.Remove(f.Name())
		}
	}()

	defer f.Close()

	if err = json.NewEncoder(f).Encode(map[string][]service{
		"services": instancesToServices(instances),
	}); err != nil {
		return err
	}

	if err = os.Rename(f.Name(), filename); err != nil {
		return err
	}

	return nil
}

type service struct {
	ID   string
	Name string
	Port int
	Tags []string
}

func instancesToServices(instances []agent.ContainerInstance) []service {
	services := []service{}

	for _, instance := range instances {
		for portName, port := range instance.ContainerConfig.Ports {
			services = append(services, service{
				ID:   fmt.Sprintf("harpoon:%s:%s", hostname(), instance.ID),
				Name: instance.ContainerConfig.Product,
				Port: int(port),
				Tags: configToTags(instance.ContainerConfig, portName),
			})
		}
	}

	return services
}

func configToTags(c agent.ContainerConfig, portName string) []string {
	return []string{
		"glimpse:harpoon",
		fmt.Sprintf("glimpse:env=%s", c.Environment),
		fmt.Sprintf("glimpse:job=%s", c.Job),
		fmt.Sprintf("glimpse:product=%s", c.Product),
		fmt.Sprintf("glimpse:service=%s", serviceize(portName)),
	}
}

func serviceize(portName string) string {
	portName = strings.ToLower(portName)

	const portPrefix = "port_"
	if strings.HasPrefix(portName, portPrefix) {
		portName = portName[len(portPrefix):]
	}

	if portName == "" {
		portName = "unknown"
	}

	return portName
}

func hostname() string {
	hostname, err := os.Hostname()
	if err != nil {
		panic("couldn't get hostname")
	}
	return hostname
}
