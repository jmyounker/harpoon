package algo

import (
	"fmt"
	"sort"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestMatch(t *testing.T) {
	for i, pair := range []struct {
		agent.ContainerConfig
		agent.HostResources
		want bool
	}{
		{
			agent.ContainerConfig{},
			agent.HostResources{},
			true,
		},
		{
			agent.ContainerConfig{Resources: agent.Resources{Mem: 1024}},
			agent.HostResources{Mem: agent.TotalReservedInt{Total: 1024, Reserved: 0}},
			true,
		},
		{
			agent.ContainerConfig{Resources: agent.Resources{Mem: 1024}},
			agent.HostResources{Mem: agent.TotalReservedInt{Total: 1024, Reserved: 1}},
			false,
		},
		{
			agent.ContainerConfig{Resources: agent.Resources{CPU: 4.0}},
			agent.HostResources{CPU: agent.TotalReserved{Total: 16.0, Reserved: 0.0}},
			true,
		},
		{
			agent.ContainerConfig{Resources: agent.Resources{CPU: 4.0}},
			agent.HostResources{CPU: agent.TotalReserved{Total: 16.0, Reserved: 12.1}},
			false,
		},
		{
			agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/container/path": "/data/1"}}},
			agent.HostResources{Volumes: []string{"/data/1"}},
			true,
		},
		{
			agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/container/path": "/data/1"}}},
			agent.HostResources{Volumes: []string{"/data/2"}},
			false,
		},
		{
			agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/path1": "/data/1", "/path3": "/data/3"}}},
			agent.HostResources{Volumes: []string{"/data/1", "/data/2", "/data/3"}},
			true,
		},
	} {
		if want, have := pair.want, match(pair.ContainerConfig, pair.HostResources); want != have {
			t.Errorf("%d: want %v, have %v", i, want, have)
		}
	}
}

func TestFilter(t *testing.T) {
	m := map[string]agent.HostResources{
		"beefy.net": agent.HostResources{
			Mem:     agent.TotalReservedInt{Total: 32000, Reserved: 128}, // 32GB total, 128MB reserved
			CPU:     agent.TotalReserved{Total: 32.0, Reserved: 1.0},     // 32 CPU total, 1 CPU reserved
			Storage: agent.TotalReserved{Total: 250 * 1e10, Reserved: 0}, // 250GB total, 0 bytes reserved
			Volumes: []string{"/data/shared", "/data/beefy"},
		},
		"wimpy.net": agent.HostResources{
			Mem:     agent.TotalReservedInt{Total: 1024, Reserved: 512},          // 1GB total, 512MB reserved
			CPU:     agent.TotalReserved{Total: 4.0, Reserved: 3.0},              // 4 CPU total, 3 CPU reserved
			Storage: agent.TotalReserved{Total: 100 * 1e10, Reserved: 70 * 1e10}, // 100GB total, 70GB reserved
			Volumes: []string{"/data/shared", "/data/wimpy"},
		},
	}

	for _, testCase := range []struct {
		agent.ContainerConfig
		want []string
	}{
		{
			agent.ContainerConfig{},
			[]string{"beefy.net", "wimpy.net"},
		},
		{
			agent.ContainerConfig{Resources: agent.Resources{Mem: 100}},
			[]string{"beefy.net", "wimpy.net"},
		},
		{
			agent.ContainerConfig{Resources: agent.Resources{Mem: 1000}},
			[]string{"beefy.net"},
		},
		{
			agent.ContainerConfig{Resources: agent.Resources{Mem: 10000}},
			[]string{"beefy.net"},
		},
		{
			agent.ContainerConfig{Resources: agent.Resources{Mem: 100000}},
			[]string{},
		},
		{
			agent.ContainerConfig{Resources: agent.Resources{CPU: 1.0}},
			[]string{"beefy.net", "wimpy.net"},
		},
		{
			agent.ContainerConfig{Resources: agent.Resources{CPU: 10.0}},
			[]string{"beefy.net"},
		},
		{
			agent.ContainerConfig{Resources: agent.Resources{CPU: 100.0}},
			[]string{},
		},
		{
			agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/container/path": "/data/shared"}}},
			[]string{"beefy.net", "wimpy.net"},
		},
		{
			agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/container/path": "/data/beefy"}}},
			[]string{"beefy.net"},
		},
		{
			agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/container/path": "/data/wimpy"}}},
			[]string{"wimpy.net"},
		},
		{
			agent.ContainerConfig{Storage: agent.Storage{Volumes: map[string]string{"/container/path": "/data/random"}}},
			[]string{},
		},
	} {
		have := filter(m, testCase.ContainerConfig)

		sort.StringSlice(have).Sort()
		sort.StringSlice(testCase.want).Sort()

		if fmt.Sprint(have) != fmt.Sprint(testCase.want) {
			t.Errorf("%v/%v: have %v, want %v", testCase.ContainerConfig.Resources, testCase.ContainerConfig.Storage.Volumes, have, testCase.want)
		}
	}
}
