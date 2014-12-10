package main

import (
	"encoding/json"
	"io/ioutil"
	"os"
	"reflect"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"

	"testing"
)

func TestServiceize(t *testing.T) {
	for input, want := range map[string]string{
		"PORT_HTTP":      "http",
		"port_http":      "http",
		"PORT_PORT_HTTP": "port_http",
		"PORT_BINDATA":   "bindata",
		"PORT_":          "unknown", // special case to prevent empty strings
		"":               "unknown",
		"_":              "_",
		"PORTA":          "porta",
		"POR_BINDATA":    "por_bindata",
		"foo":            "foo",
	} {
		if have := serviceize(input); want != have {
			t.Errorf("%s: want %q, have %q", input, want, have)
		}
	}
}

func TestConfigToTags(t *testing.T) {
	c := agent.ContainerConfig{
		Environment: "prod",
		Job:         "request-processor",
		Product:     "android",
		Ports:       map[string]uint16{"PORT_HTTP": 31234},
	}

	want := []string{
		"glimpse:harpoon",
		"glimpse:env=prod",
		"glimpse:job=request-processor",
		"glimpse:product=android",
		"glimpse:service=http",
	}

	if have := configToTags(c, "PORT_HTTP"); !reflect.DeepEqual(want, have) {
		t.Errorf("want\n\t%#v, have \n\t%#v", want, have)
	}
}

func TestInstancesToServices(t *testing.T) {
	instance := agent.ContainerInstance{
		ID: "my-request-processor-1",
		ContainerConfig: agent.ContainerConfig{
			Job:         "request-processor",
			Environment: "prod",
			Product:     "android",
			Ports:       map[string]uint16{"PORT_HTTP": 31234},
		},
	}

	instances := map[string]agent.ContainerInstance{instance.ID: instance}

	want := []service{
		service{
			ID:   "harpoon:" + hostname() + ":my-request-processor-1",
			Name: "android",
			Port: 31234,
			Tags: configToTags(instance.ContainerConfig, "PORT_HTTP"),
		},
	}

	if have := instancesToServices(instances); !reflect.DeepEqual(want, have) {
		t.Errorf("want\n\t%#v, have \n\t%#v", want, have)
	}
}

func TestWriteInstances(t *testing.T) {
	filename := "test-write-service-discovery.json"

	instance := agent.ContainerInstance{
		ID: "my-request-processor-1",
		ContainerConfig: agent.ContainerConfig{
			Job:         "request-processor",
			Environment: "prod",
			Product:     "android",
			Ports:       map[string]uint16{"PORT_HTTP": 31234},
		},
	}

	instances := map[string]agent.ContainerInstance{instance.ID: instance}

	if err := writeInstances(filename, instances); err != nil {
		t.Fatal(err)
	}

	buf, err := ioutil.ReadFile(filename)
	if err != nil {
		os.Remove(filename)
		t.Fatalf("reading written file: %s", err)
	}

	os.Remove(filename)

	var have map[string][]service
	if err := json.Unmarshal(buf, &have); err != nil {
		t.Fatal(err)
	}

	if want := map[string][]service{"services": instancesToServices(instances)}; !reflect.DeepEqual(want, have) {
		t.Errorf("want\n\t%#v, have \n\t%#v", want, have)
	}
}
