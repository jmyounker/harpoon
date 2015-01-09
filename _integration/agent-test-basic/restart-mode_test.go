package agent_test

import (
	"testing"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

func TestNoRestartWhenStoppedDoesNotRestart(t *testing.T) {
	cf := newContainerFixture(t, "stopped-job-does-not-restart(restart=no)")
	cf.config.Restart = agent.NoRestart

	cf.create(agent.ContainerStatusCreated)
	cf.start(agent.ContainerStatusRunning)
	cf.stop(agent.ContainerStatusFinished)
	// Container should not restart.
	wc := cf.wait(5*time.Second, agent.ContainerStatusRunning)
	w := <-wc
	if w.Err == nil {
		t.Fatal("Container should not restart.")
	}
	cf.destroy()
}

func TestNoRestartNormalWithNormalExitDoesNotRestart(t *testing.T) {
	cf := newContainerFixture(t, "finished-job-does-not-restart(restart=no)")
	cf.config.Command.Exec = []string{"bin/warhead", "-batch-mode", "-run-time", "1s", "-exit-code", "0"}
	cf.config.Restart = agent.NoRestart

	cf.create(agent.ContainerStatusCreated)
	cf.start(agent.ContainerStatusRunning)
	// Container should finish.
	wc := cf.wait(5*time.Second, agent.ContainerStatusFinished)
	w := <-wc
	if w.Err != nil {
		t.Fatal("Container did not exit with state Finished")
	}
	// Destroy should fail if the container is restarting.
	cf.destroy()
}

func TestNoRestartWithFailureDoesNotRestart(t *testing.T) {
	cf := newContainerFixture(t, "failed-job-does-not-restart(restart=no)")
	cf.config.Command.Exec = []string{"bin/warhead", "-batch-mode", "-run-time", "1s", "-exit-code", "2"}
	cf.config.Restart = agent.NoRestart

	cf.create(agent.ContainerStatusCreated)
	stopEvent := cf.watchEvents()
	defer stopEvent()
	cf.start(agent.ContainerStatusRunning)
	// Container should finish.
	wc := cf.wait(5*time.Second, agent.ContainerStatusFailed)
	w := <-wc
	if w.Err != nil {
		t.Fatal("Container did not exit with state Failed")
	}
	// Destroy should fail if the container is restarting.
	cf.destroy()
}

//func TestAlwaysRestartDoesNotAffectStoppedJobs(t *testing.T) {
//	cf := newContainerFixture(t, "stopped-job-does-not-restart(restart=always)")
//	cf.config.Restart = agent.AlwaysRestart
//
//	cf.create(agent.ContainerStatusCreated)
//	cf.start(agent.ContainerStatusRunning)
//	cf.stop(agent.ContainerStatusFinsihed)
//	// Container should not restart
//	wc := cf.wait(5 * time.Second, agent.ContainerStatusRunning)
//	w := <- wc
//	if w.Err == nil {
//		t.Fatal("Container should not have been restarted")
//	}
//	cf.destroy()
//}
//
//func TestAlwaysRestartWithFinishedJobRestarts(t * testing.T) {
//	cf := newContainerFixture(t, "finished-job-restarts(restart=always)")
//	cf.config.Command.Exec = []string{"bin/warhead", "-batch-mode", "-run-time", "1s", "-exit-code", "1"}
//	cf.config.Restart = agent.AlwaysRestart
//
//	cf.create(agent.ContainerStatusCreated)
//	cf.start(agent.ContainerStatusRunning)
//	// Container should fail
//	wc := cf.wait(5 * time.Second, agent.ContainerStatusFinished)
//	w := <- wc
//	if w.Err != nil {
//		t.Fatal("Container did not exit with state Finished")
//	}
//	// Container should restart
//	wc := cf.wait(5 * time.Second, agent.ContainerStatusRunning)
//	w := <- wc
//	if w.Err != nil {
//		t.Fatal("Container did not restart")
//	}
//
//	cf.stop(agent.ContainerStatusFinished)
//	cf.destroy()
//}
//
//func TestAlwaysRestartWithFailedJobRestarts(t * testing.T) {
//	cf := newContainerFixture(t, "failed-job-restarts(restart=always")
//	cf.config.Command.Exec = []string{"bin/warhead", "-batch-mode", "-run-time", "1s", "-exit-code", "0"}
//	cf.config.Restart = agent.AlwaysRestart
//
//	cf.create(agent.ContainerStatusCreated)
//	cf.start(agent.ContainerStatusRunning)
//	// Container should fail
//	wc := cf.wait(5 * time.Second, agent.ContainerStatusFailed)
//	w := <- wc
//	if w.Err != nil {
//		t.Fatal("Container did not exit with state Failed")
//	}
//	// Container should restart
//	wc := cf.wait(5 * time.Second, agent.ContainerStatusRunning)
//	w := <- wc
//	if w.Err != nil {
//		t.Fatal("Container did not restart")
//	}
//
//	cf.stop(agent.ContainerStatusFinished)
//	cf.destroy()
//}
//
//func TestOnFailureRestartDoesNotAffectStoppedJobs(t *testing.T) {
//	cf := newContainerFixture(t, "stopped-job-does-not-restart(restart=on-failure)")
//	cf.config.Restart = agent.OnFailureRestart
//
//	cf.create(agent.ContainerStatusCreated)
//	cf.start(agent.ContainerStatusRunning)
//	cf.stop(agent.ContainerStatusFinsihed)
//	// Container should not restart
//	wc := cf.wait(5 * time.Second, agent.ContainerStatusRunning)
//	w := <- wc
//	if w.Err == nil {
//		t.Fatal("Container should not have been restarted")
//	}
//	// If this were restarting then destroy would not work.
//	cf.destroy()
//}
//
//func TestOnFailureRestartWithFinishedJobDoesNotRestart(t * testing.T) {
//	cf := newContainerFixture(t, "finished-job-does-not-restart(restart=on-finished")
//	cf.config.Command.Exec = []string{"bin/warhead", "-batch-mode", "-run-time", "1s", "-exit-code", "0"}
//	cf.config.Restart = agent.OnFailureRestart
//
//	cf.create(agent.ContainerStatusCreated)
//	cf.start(agent.ContainerStatusRunning)
//	// Container should fail
//	wc := cf.wait(5 * time.Second, agent.ContainerStatusFinished)
//	w := <- wc
//	if w.Err != nil {
//		t.Fatal("Container did not exit with state Finished")
//	}
//	// If this were restarting then destroy would not work.
//	cf.destroy()
//}
//
//func TestOnFailureRestartWithFailedJobRestarts(t * testing.T) {
//	cf := newContainerFixture(t, "failed-job-restarts(restart=on-failure")
//	cf.config.Command.Exec = []string{"bin/warhead", "-batch-mode", "-run-time", "1s", "-exit-code", "1"}
//	cf.config.Restart = agent.OnFailureRestart
//
//	cf.create(agent.ContainerStatusCreated)
//	cf.start(agent.ContainerStatusRunning)
//	// Container should fail
//	wc := cf.wait(5 * time.Second, agent.ContainerStatusFailed)
//	w := <- wc
//	if w.Err != nil {
//		t.Fatal("Container did not exit with state Failed")
//	}
//	// Container should restart
//	wc := cf.wait(5 * time.Second, agent.ContainerStatusRunning)
//	w := <- wc
//	if w.Err != nil {
//		t.Fatal("Container did not restart")
//	}
//
//	cf.stop(agent.ContainerStatusFinished)
//	cf.destroy()
//}
//
