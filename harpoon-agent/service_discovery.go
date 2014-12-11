package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

type serviceDiscovery interface {
	Update(map[string]agent.ContainerInstance) error
}

type nopServiceDiscovery struct{}

func (sd nopServiceDiscovery) Update(_ map[string]agent.ContainerInstance) error { return nil }

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

func (sd consulServiceDiscovery) Update(instances map[string]agent.ContainerInstance) (err error) {
	defer func(begin time.Time) {
		if err == nil {
			incSDUpdateSuccessful(time.Since(begin))
		} else {
			incSDUpdateFailed(time.Since(begin))
		}
	}(time.Now())

	if err := writeInstances(sd.filename, instances); err != nil {
		return err
	}

	if buf, err := exec.Command(sd.reload[0], sd.reload[1:]...).CombinedOutput(); err != nil {
		log.Printf("service discovery reload failed\n%s", string(buf))
		return err
	}

	return nil
}

func writeInstances(filename string, instances map[string]agent.ContainerInstance) (err error) {
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

func instancesToServices(instances map[string]agent.ContainerInstance) []service {
	services := serviceSlice{}

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

	sort.Sort(services)

	return services
}

func configToTags(c agent.ContainerConfig, portName string) []string {
	return []string{
		"glimpse:provider=harpoon",
		fmt.Sprintf("glimpse:product=%s", c.Product),
		fmt.Sprintf("glimpse:env=%s", c.Environment),
		fmt.Sprintf("glimpse:job=%s", c.Job),
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

type serviceSlice []service

func (a serviceSlice) Len() int           { return len(a) }
func (a serviceSlice) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a serviceSlice) Less(i, j int) bool { return a[i].ID < a[j].ID }
