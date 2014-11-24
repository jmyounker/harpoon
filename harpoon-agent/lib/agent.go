package agent

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

// Agent describes the agent API (v0) spec in the Go domain.
//
// The only notable change from the spec doc is that `log` is only available
// as a stream. Clients are expected to stop the stream after enough log lines
// have been received.
type Agent interface {
	Endpoint() string
	Put(containerID string, containerConfig ContainerConfig) error                                                  // PUT /containers/{id}
	Get(containerID string) (ContainerInstance, error)                                                              // GET /containers/{id}
	Start(containerID string) error                                                                                 // POST /containers/{id}/start
	Stop(containerID string) error                                                                                  // POST /containers/{id}/stop
	Replace(newContainerID, oldContainerID string) error                                                            // PUT /containers/{newID}?replace={oldID}
	Destroy(containerID string) error                                                                               // DELETE /containers/{id}
	Containers() (map[string]ContainerInstance, error)                                                              // GET /containers
	Events() (<-chan StateEvent, Stopper, error)                                                                    // GET /containers with request header Accept: text/event-stream
	Log(containerID string, history int) (<-chan string, Stopper, error)                                            // GET /containers/{id}/log?history=10
	Resources() (HostResources, error)                                                                              // GET /resources
	Wait(containerID string, statuses map[ContainerStatus]struct{}, timeout time.Duration) (ContainerStatus, error) // Waits for event with one of the statuses
}

// ContainerConfig describes the information necessary to start a container on
// an agent.
type ContainerConfig struct {
	ArtifactURL string            `json:"artifact_url"`
	Ports       map[string]uint16 `json:"ports"`
	Env         map[string]string `json:"env"`
	Command     `json:"command"`
	Resources   `json:"resources"`
	Storage     `json:"storage"`
	Grace       `json:"grace"`
	Restart     `json:"restart"`
}

// Valid performs a validation check, to ensure invalid structures may be
// detected as early as possible.
func (c ContainerConfig) Valid() error {
	var errs []string

	if _, err := url.Parse(c.ArtifactURL); err != nil {
		errs = append(errs, fmt.Sprintf("artifact URL %q invalid: %s", c.ArtifactURL, err))
	}

	if err := c.Command.Valid(); err != nil {
		errs = append(errs, fmt.Sprintf("command invalid: %s", err))
	}

	if err := c.Resources.Valid(); err != nil {
		errs = append(errs, fmt.Sprintf("resources invalid: %s", err))
	}

	if err := c.Storage.Valid(); err != nil {
		errs = append(errs, fmt.Sprintf("storage invalid: %s", err))
	}

	if err := c.Grace.Valid(); err != nil {
		errs = append(errs, fmt.Sprintf("grace periods invalid: %s", err))
	}

	if err := c.Restart.Valid(); err != nil {
		errs = append(errs, fmt.Sprintf("restart policy invalid: %s", err))
	}

	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}

	return nil
}

// Command describes how to start a binary.
type Command struct {
	WorkingDir string   `json:"working_dir"`
	Exec       []string `json:"exec"`
}

// Valid performs a validation check, to ensure invalid structures may be
// detected as early as possible.
func (c Command) Valid() error {
	var errs []string
	if len(c.Exec) <= 0 {
		errs = append(errs, "exec (command to run, as array) not specified")
	}
	if len(c.WorkingDir) <= 0 {
		errs = append(errs, "working dir (string) not specified")
	}
	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}
	return nil
}

// Resources describes resource limits for a container.
type Resources struct {
	Mem uint64  `json:"mem"` // MB
	CPU float64 `json:"cpu"` // fractional CPUs
	FD  uint64  `json:"fd"`  // file descriptor hard limit
}

// Valid performs a validation check, to ensure invalid structures may be
// detected as early as possible.
func (r Resources) Valid() error {
	var errs []string
	if r.CPU <= 0.0 {
		errs = append(errs, "cpu (floating point fractional CPUs) not specified or zero")
	}
	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}
	return nil
}

// Storage describes storage requirements for a container.
type Storage struct {
	Tmp     map[string]int    `json:"tmp"`     // container path: max alloc megabytes (-1 for unlimited)
	Volumes map[string]string `json:"volumes"` // container path: host path
}

// Valid performs a validation check, to ensure invalid structures may be
// detected as early as possible.
func (s Storage) Valid() error {
	// TODO: what constitutes invalid storage specification?
	return nil
}

// Grace describes how many seconds the scheduler should wait for a container
// to start up and shut down before giving up on that operation. Containers
// that don't shut down within the shutdown window may be subject to a more
// forceful kill.
type Grace struct {
	Startup  JSONDuration `json:"startup"`
	Shutdown JSONDuration `json:"shutdown"`
}

const (
	minStartupDuration  = 250 * time.Millisecond
	maxStartupDuration  = 30 * time.Second
	minShutdownDuration = 250 * time.Millisecond
	maxShutdownDuration = 10 * time.Second
)

// Valid performs a validation check, to ensure invalid structures may be
// detected as early as possible.
func (g Grace) Valid() error {
	var errs []string

	if g.Startup.Duration < minStartupDuration || g.Startup.Duration > maxStartupDuration {
		errs = append(errs, fmt.Sprintf("startup (%s) must be between %s and %s", g.Startup, minStartupDuration, maxStartupDuration))
	}

	if g.Shutdown.Duration < minShutdownDuration || g.Shutdown.Duration > maxShutdownDuration {
		errs = append(errs, fmt.Sprintf("shutdown (%s) must be between %s and %s", g.Shutdown, minShutdownDuration, maxShutdownDuration))
	}

	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}

	return nil
}

// Restart describes the restart policy of a container in an agent.
//
// Docker provides some inspiration here.
// http://blog.docker.com/2014/08/announcing-docker-1-2-0/
type Restart string

const (
	// NoRestart indicates that the container won't be restarted if it dies
	NoRestart Restart = "no"

	// OnFailureRestart indicates that the container will be restarted
	// only if it exits with a non-zero status
	OnFailureRestart = "on-failure"

	// AlwaysRestart indicates that the container will be restarted
	// no matter what exit code is returned
	AlwaysRestart = "always"
)

// Valid performs a validation check, to ensure invalid structures may be
// detected as early as possible.
func (r Restart) Valid() error {
	switch r {
	case NoRestart, OnFailureRestart, AlwaysRestart:
	default:
		return fmt.Errorf(
			"%q should be %s, %s or %s",
			r,
			NoRestart,
			OnFailureRestart,
			AlwaysRestart,
		)
	}

	return nil
}

// StateEvent is returned whenever a container changes state. It reflects the
// changed container and the current host resources (post-change).
type StateEvent struct {
	Resources  HostResources                `json:"resources"`
	Containers map[string]ContainerInstance `json:"containers"`
}

// HostResources are returned by agents and reflect their current state.
type HostResources struct {
	Mem     TotalReservedInt `json:"mem"`     // MB
	CPU     TotalReserved    `json:"cpus"`    // whole CPUs
	Storage TotalReservedInt `json:"storage"` // Bytes
	Volumes []string         `json:"volumes"`
}

// TotalReserved encodes the total scalar amount of an arbitrary resource
// (total) and the amount of it that's currently in-use (reserved).
type TotalReserved struct {
	Total    float64 `json:"total"`
	Reserved float64 `json:"reserved"`
}

// TotalReservedInt encodes the total scalar amount of an arbitrary resource
// (total) and the amount of it that's currently in-use (reserved) as integer values.
type TotalReservedInt struct {
	Total    uint64 `json:"total"`
	Reserved uint64 `json:"reserved"`
}

// Stopper describes anything that can be stopped, such as an event stream.
// TODO(pb): it would be nice to use a different idiom, and delete this.
type Stopper interface {
	Stop()
}

// ContainerInstance describes the state of an individual container running on
// an agent machine. In scheduler terminology, it always describes one
// instance of a task.
//
// Implementer's note: in a departure from the original container API spec
// draft, the agent's event stream should send container instances directly on
// changes, rather than have a separate "event" data type and some kind of
// mapping. That implies objects in the event stream represent complete
// current states, rather than transitions. I believe that will prove more
// sustainable.
//
// Additional note: while an agent requires only that container IDs are unique
// in its singular domain, we extend that constraint and declare that IDs are
// globally unique in the entire scheduling domain. This works only because
// container IDs are provided with the PUT/POST, rather than assigned by the
// agent.
type ContainerInstance struct {
	ID                    string `json:"container_id"`
	ContainerStatus       `json:"status"`
	ContainerConfig       `json:"config"`
	ContainerProcessState `json:"process_state"`
}

// ContainerStatus describes the current state of a container in an agent. The
// enumerated statuses, below, are a really quick first draft, and are
// probably underspecified.
//
// The Aurora state machine provides some inspiration here.
// https://github.com/soundcloud/harpoon/blob/master/doc/schedulers.md#aurora
type ContainerStatus string

const (
	// ContainerStatusCreated indicates the container has been successfully
	// PUT on the agent, but hasn't yet been started. Once a container leaves
	// the created state, it will never come back.
	ContainerStatusCreated ContainerStatus = "created"

	// ContainerStatusRunning indicates the container is succesfully running
	// from the perspective of the agent. It implies nothing about the
	// healthiness of the process.
	ContainerStatusRunning ContainerStatus = "running"

	// ContainerStatusFailed indicates the container has exited with a nonzero
	// return code. In most cases, this is a very short-lived state, as the
	// agent will restart the container.
	ContainerStatusFailed ContainerStatus = "failed"

	// ContainerStatusFinished indicates the container has exited successfully
	// with a zero return code. In most cases, this will be a long-lived
	// state, as the agent will not restart the container. (We should probably
	// think about if and how to reap finished containers.)
	ContainerStatusFinished ContainerStatus = "finished"

	// ContainerStatusDeleted is a special meta-state used only in event
	// signaling. It's sent to event stream subscribers when a container is
	// successfully deleted. It should never be stored, only part of an event.
	ContainerStatusDeleted ContainerStatus = "deleted"
)

// JSONDuration allows specification of time.Duration as strings in JSON-
// serialized structs. For example, "250ms", "5s", "30m".
type JSONDuration struct{ time.Duration }

// String implements the Stringer interface, for convenience.
func (d JSONDuration) String() string { return d.Duration.String() }

// MarshalJSON implements the json.Marshaler interface.
func (d JSONDuration) MarshalJSON() ([]byte, error) {
	return []byte(fmt.Sprintf(`"%s"`, d.Duration.String())), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (d *JSONDuration) UnmarshalJSON(buf []byte) error {
	dur, err := time.ParseDuration(strings.Trim(string(buf), `"`))
	if err != nil {
		return err
	}

	d.Duration = dur

	return nil
}

// ContainerProcessState contains the state of a container.
type ContainerProcessState struct {
	// Up signals whether the container process is running.
	Up bool `json:"up"`

	// Restarting signals whether the container process will be restarted after
	// exit. If both Up and Restarting are false, the container process is down
	// and will not be restarted.
	Restarting bool `json:"restarting"`

	// Err records a non-recoverable error which prevented the container from
	// starting. It will only be set if both Up and Restarting are false.
	Err string `json:"err,omitempty"`

	// ContainerExitStatus contains the last exit status of the container. It
	// will only be present if Up is false.
	ContainerExitStatus `json:"container_exit_status,omitempty"`

	// Restarts is a counter of how often the container has been restarted
	Restarts uint `json:"restarts"`

	// OOMs is a counter of how often the container has been killed for exceeding
	// its memory limit.
	OOMs uint `json:"ooms"`

	ContainerMetrics `json:"container_metrics"`
}

// ContainerExitStatus contains the exit status of a container.
type ContainerExitStatus struct {
	// Exited is true when the container exited on its own, or in response to
	// handling a signal. ExitStatus will be >= 0 when Exited is true.
	Exited     bool `json:"exited,omitempty"`
	ExitStatus int  `json:"exit_status,omitempty"`

	// Signaled is true when the container was killed with a signal. Signal
	// will be > 0 when Signaled is true.
	Signaled bool `json:"signaled,omitempty"`
	Signal   int  `json:"signal,omitempty"`

	// OOMed is true if the container was killed for exceeding its memory
	// limit.
	OOMed bool `json:"oomed,omitempty"`
}

// ContainerMetrics contains detailed historical information about a unique
// container. ContainerMetrics are tracked across restarts.
type ContainerMetrics struct {
	CPUTime     uint64 `json:"cpu_time"`     // total counter of cpu time
	MemoryUsage uint64 `json:"memory_usage"` // memory usage in bytes
	MemoryLimit uint64 `json:"memory_limit"` // memory limit in bytes
}
