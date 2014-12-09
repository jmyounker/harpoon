package scheduler_test

import (
	"flag"
	"fmt"
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
	"github.com/soundcloud/harpoon/harpoonctl/scheduler"
)

var (
	schedulerURL = flag.String("integ.scheduler.url", "", "integration test scheduler url")
	agentURL     = flag.String("integ.agent.url", "", "integration test agent url")
)

func TestBasicTaskSchedule(t *testing.T) {
	clientAgent, err := agent.NewClient(*agentURL)
	if err != nil {
		t.Fatal(err)
	}

	res, err := clientAgent.Resources()
	if err != nil {
		t.Fatal(err)
	}

	var (
		mem  = res.Mem.Total
		cpus = res.CPU.Total
	)

	var cfg = configstore.JobConfig{
		Job:         "test",
		Scale:       3,
		Environment: "Env",
		Product:     "Product",
		ContainerConfig: agent.ContainerConfig{
			ArtifactURL: "http://asset-host.test/busybox.tar.gz",
			Command: agent.Command{
				WorkingDir: "/bin",
				Exec:       []string{"./true"},
			},
			Resources: agent.Resources{
				Mem: mem / 3,
				CPU: cpus / 3,
			},
			Grace: agent.Grace{
				Startup:  agent.JSONDuration{time.Second},
				Shutdown: agent.JSONDuration{time.Second},
			},
			Restart: "no",
		},
	}

	if _, err := scheduler.Schedule(cfg); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	if err := validateContainers(clientAgent, 3, cfg); err != nil {
		t.Fatal(err)
	}

	if _, err = scheduler.UnscheduleConfig(cfg); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	if err := validateContainers(clientAgent, 0, configstore.JobConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestUnscheduleNonexistentTask(t *testing.T) {
	clientAgent, err := agent.NewClient(*agentURL)
	if err != nil {
		t.Fatal(err)
	}

	var cfg = configstore.JobConfig{
		Job:         "test",
		Scale:       3,
		Environment: "Env",
		Product:     "Product",
		ContainerConfig: agent.ContainerConfig{
			ArtifactURL: "http://asset-host.test/busybox.tar.gz",
			Command: agent.Command{
				WorkingDir: "/bin",
				Exec:       []string{"./true"},
			},
			Resources: agent.Resources{
				Mem: 100,
				CPU: 1,
			},
			Grace: agent.Grace{
				Startup:  agent.JSONDuration{time.Second},
				Shutdown: agent.JSONDuration{time.Second},
			},
			Restart: "no",
		},
	}

	if _, err = scheduler.UnscheduleConfig(cfg); err == nil {
		t.Fatal("unscheduling unexisting config should return error")
	}

	time.Sleep(time.Second)

	if err := validateContainers(clientAgent, 0, configstore.JobConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestImpossibleTasks(t *testing.T) {
	clientAgent, err := agent.NewClient(*agentURL)
	if err != nil {
		t.Fatal(err)
	}

	res, err := clientAgent.Resources()
	if err != nil {
		t.Fatal(err)
	}

	var (
		mem  = res.Mem.Total
		cpus = res.CPU.Total
	)

	var cfg = configstore.JobConfig{
		Job:   "test",
		Scale: 2,
		ContainerConfig: agent.ContainerConfig{
			ArtifactURL: "http://asset-host.test/busybox.tar.gz",
			Command: agent.Command{
				WorkingDir: "/bin",
				Exec:       []string{"./true"},
			},
			Resources: agent.Resources{
				Mem: 32,
				CPU: 0.5,
			},
			Grace: agent.Grace{
				Startup:  agent.JSONDuration{time.Second},
				Shutdown: agent.JSONDuration{time.Second},
			},
			Restart: "no",
		},
	}

	type testCase struct {
		mem  uint64
		cpus float64
	}

	for i, input := range []testCase{
		{mem + 1, cpus},
		{mem, cpus + 1},
		{mem + 1, cpus + 1},
	} {
		cfg.Resources.CPU = input.cpus
		cfg.Resources.Mem = input.mem

		if _, err := scheduler.Schedule(cfg); err == nil {
			t.Fatalf("%d: incorrect scheduling", i)
		}

		if err := validateContainers(clientAgent, 0, configstore.JobConfig{}); err != nil {
			t.Fatalf("%d error: %v", i, err)
		}

		if _, err = scheduler.UnscheduleConfig(cfg); err == nil {
			t.Fatalf("%d: incorrect unscheduling", i)
		}

		if err := validateContainers(clientAgent, 0, configstore.JobConfig{}); err != nil {
			t.Fatalf("%d error: %v", i, err)
		}
	}
}

func TestDirectScheduleOnAgent(t *testing.T) {
	client, err := agent.NewClient(*agentURL)
	if err != nil {
		t.Fatal(err)
	}

	var cfg = agent.ContainerConfig{
		ArtifactURL: "http://asset-host.test/busybox.tar.gz",
		Command: agent.Command{
			WorkingDir: "/bin",
			Exec:       []string{"./true"},
		},
		Resources: agent.Resources{
			Mem: 32,
			CPU: 1,
		},
		Grace: agent.Grace{
			Startup:  agent.JSONDuration{time.Second},
			Shutdown: agent.JSONDuration{time.Second},
		},
		Restart: "no",
	}

	statuses := map[agent.ContainerStatus]struct{}{
		agent.ContainerStatusCreated: struct{}{},
	}
	wc := client.Wait("basic-test", statuses, 2*time.Second)

	if err := client.Put("basic-test", cfg); err != nil {
		t.Fatal(err)
	}

	w := <-wc
	if w.Err != nil {
		t.Fatal(w.Err)
	}

	statuses = map[agent.ContainerStatus]struct{}{
		agent.ContainerStatusDeleted: struct{}{},
	}
	w = <-client.Wait("basic-test", statuses, 2*time.Second)
	if w.Err != nil {
		t.Fatal(w.Err)
	}

	if w.Status != agent.ContainerStatusDeleted {
		t.Fatalf("incorrect status %s", w.Status)
	}
}

func TestTaskConsumesAllAllowedResources(t *testing.T) {
	clientAgent, err := agent.NewClient(*agentURL)
	if err != nil {
		t.Fatal(err)
	}

	res, err := clientAgent.Resources()
	if err != nil {
		t.Fatal(err)
	}

	var (
		mem  = res.Mem.Total
		cpus = res.CPU.Total
	)

	var cfg = configstore.JobConfig{
		Job:         "test",
		Scale:       5,
		Environment: "Env",
		Product:     "Product",
		ContainerConfig: agent.ContainerConfig{
			ArtifactURL: "http://asset-host.test/busybox.tar.gz",
			Command: agent.Command{
				WorkingDir: "/bin",
				Exec:       []string{"./true"},
			},
			Resources: agent.Resources{
				Mem: mem / 3,
				CPU: cpus / 3,
			},
			Grace: agent.Grace{
				Startup:  agent.JSONDuration{time.Second},
				Shutdown: agent.JSONDuration{time.Second},
			},
			Restart: "no",
		},
	}

	if _, err := scheduler.Schedule(cfg); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	if err := validateContainers(clientAgent, 3, cfg); err != nil {
		t.Fatal(err)
	}

	if _, err = clientScheduler.Unschedule(cfg); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	if err := validateContainers(clientAgent, 0, configstore.JobConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestTaskScheduledWhenResourcesAreFree(t *testing.T) {
	clientScheduler, err := scheduler.NewClient(*schedulerURL)
	if err != nil {
		t.Fatal(err)
	}

	clientAgent, err := agent.NewClient(*agentURL)
	if err != nil {
		t.Fatal(err)
	}

	res, err := clientAgent.Resources()
	if err != nil {
		t.Fatal(err)
	}

	var (
		mem  = res.Memory.Total
		cpus = res.CPUs.Total
	)

	var firstCfg = configstore.JobConfig{
		Job:         "test",
		Scale:       5,
		Environment: "Env",
		Product:     "Product",
		ContainerConfig: agent.ContainerConfig{
			ArtifactURL: "http://asset-host.test/busybox.tar.gz",
			Command: agent.Command{
				WorkingDir: "/bin",
				Exec:       []string{"./true"},
			},
			Resources: agent.Resources{
				Memory: mem / 3,
				CPUs:   cpus / 3,
			},
			Grace: agent.Grace{
				Startup:  agent.JSONDuration{time.Second},
				Shutdown: agent.JSONDuration{time.Second},
			},
			Restart: "no",
		},
	}

	secondCfg := firstCfg
	secondCfg.Scale = 4
	secondCfg.ContainerConfig.Resources = agent.Resources{Memory: mem / 2, CPUs: cpus / 2}

	if _, err := clientScheduler.Schedule(firstCfg); err != nil {
		t.Fatal(err)
	}

	if _, err := clientScheduler.Schedule(secondCfg); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second * 2)

	if err := validateContainers(clientAgent, 3, firstCfg); err != nil {
		t.Fatal(err)
	}

	if _, err = clientScheduler.Unschedule(firstCfg); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second * 2)

	if err := validateContainers(clientAgent, 2, secondCfg); err != nil {
		t.Fatal(err)
	}

	if _, err = clientScheduler.Unschedule(secondCfg); err != nil {
		t.Fatal(err)
	}

	time.Sleep(time.Second)

	if err := validateContainers(clientAgent, 0, configstore.JobConfig{}); err != nil {
		t.Fatal(err)
	}
}

func TestScheduleMultipleIndependentTasksThatFit(t *testing.T) {
	clientScheduler, err := scheduler.NewClient(*schedulerURL)
	if err != nil {
		t.Fatal(err)
	}

	clientAgent, err := agent.NewClient(*agentURL)
	if err != nil {
		t.Fatal(err)
	}

	res, err := clientAgent.Resources()
	if err != nil {
		t.Fatal(err)
	}

	var (
		mem  = res.Memory.Total
		cpus = res.CPUs.Total
	)

	var cfg = configstore.JobConfig{
		Job:         "test",
		Scale:       1,
		Environment: "Env",
		Product:     "Product",
		ContainerConfig: agent.ContainerConfig{
			ArtifactURL: "http://asset-host.test/busybox.tar.gz",
			Command: agent.Command{
				WorkingDir: "/bin",
				Exec:       []string{"./true"},
			},
			Resources: agent.Resources{
				Memory: mem / 10,
				CPUs:   cpus / 10,
			},
			Grace: agent.Grace{
				Startup:  agent.JSONDuration{time.Second},
				Shutdown: agent.JSONDuration{time.Second},
			},
			Restart: "no",
		},
	}

	for i := 0; i < 7; i++ {
		cfg.Product = fmt.Sprintf("product%d", i)
		if _, err := clientScheduler.Schedule(cfg); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(time.Second * 2)

	if err := validateContainers(clientAgent, 7, configstore.JobConfig{}); err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 7; i++ {
		cfg.Product = fmt.Sprintf("product%d", i)
		if _, err := clientScheduler.Unschedule(cfg); err != nil {
			t.Fatal(err)
		}
	}

	time.Sleep(time.Second)

	if err := validateContainers(clientAgent, 0, configstore.JobConfig{}); err != nil {
		t.Fatal(err)
	}
}

// validateContainers checks if the expected number of containers are present,
// and ensures that all are either running or finished
func validateContainers(client agent.Agent, expectedCount int, cfg configstore.JobConfig) error {
	containers, err := client.Containers()
	if err != nil {
		return err
	}

	if expected, actual := expectedCount, len(containers); expected != actual {
		return fmt.Errorf("invalid instance: expected %d != actual %d", expected, actual)
	}

	if cfg.Scale == 0 {
		return nil
	}

	containerCount := 0
	for i := 0; i < cfg.Scale; i++ {
		var (
			containerName = fmt.Sprintf("%s-%d", cfg.Hash(), i)
			instance, ok  = containers[containerName]
		)

		if !ok {
			continue
		}

		switch instance.ContainerStatus {
		case agent.ContainerStatusFinished, agent.ContainerStatusRunning:
		default:
			return fmt.Errorf("%s : incorrect status %v", containerName, instance.ContainerStatus)
		}

		containerCount++
	}

	if containerCount != expectedCount {
		return fmt.Errorf("wrong count of containers: actual %d !=  expected %d", containerCount, expectedCount)
	}

	return nil
}
