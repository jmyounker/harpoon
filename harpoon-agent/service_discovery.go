package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type serviceDiscovery interface {
	Update([]agent.ContainerInstance) error
}

type consulServiceDiscovery struct {
	filename string
	reload   []string
}

func newConsulServiceDiscovery(filename, reloadCommand string) serviceDiscovery {
	args := strings.Split(reloadCommand, " ")
	if len(args) <= 0 {
		panic(fmt.Sprintf("invalid reload command %q", reloadCommand))
	}

	return &consulServiceDiscovery{
		filename: filename,
		reload:   args,
	}
}

func (d consulServiceDiscovery) Update(instances []agent.ContainerInstance) error {
	if err := writeInstances(d.filename, instances); err != nil {
		return err
	}

	if buf, err := exec.Command(d.reload[0], d.reload[1:]...).CombinedOutput(); err != nil {
		log.Printf("service discovery reload failed\n%s", string(buf))
		return err
	}

	return nil
}

func writeInstances(filename string, instances []agent.ContainerInstance) (err error) {
	if filename == "" {
		return fmt.Errorf("no file provided")
	}

	// Ensure that the temp file is in the same filesystem as the registry
	// save file so that os.Rename() never crosses a filesystem boundary.
	f, err := ioutil.TempFile(filepath.Dir(filename), "harpoon-scheduler-service-discovery")
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

	if err := f.Sync(); err != nil {
		return err
	}

	f.Close() // double close is OK, I think

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
