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
	Put(containerID string, containerConfig ContainerConfig) error       // PUT /containers/{id}
	Get(containerID string) (ContainerInstance, error)                   // GET /containers/{id}
	Start(containerID string) error                                      // POST /containers/{id}/start
	Stop(containerID string) error                                       // POST /containers/{id}/stop
	Replace(newContainerID, oldContainerID string) error                 // PUT /containers/{newID}?replace={oldID}
	Delete(containerID string) error                                     // DELETE /containers/{id}
	Containers() (map[string]ContainerInstance, error)                   // GET /containers
	Events() (<-chan map[string]ContainerInstance, Stopper, error)       // GET /containers with request header Accept: text/event-stream
	Log(containerID string, history int) (<-chan string, Stopper, error) // GET /containers/{id}/log?history=10
	Resources() (HostResources, error)                                   // GET /resources
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
	Memory int     `json:"mem"`  // MB
	CPUs   float64 `json:"cpus"` // fractional CPUs
}

// Valid performs a validation check, to ensure invalid structures may be
// detected as early as possible.
func (r Resources) Valid() error {
	var errs []string
	if r.Memory <= 0 {
		errs = append(errs, "mem (integer MB) not specified or zero")
	}
	if r.CPUs <= 0.0 {
		errs = append(errs, "cpus (floating point fractional CPUs) not specified or zero")
	}
	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}
	return nil
}

// Storage describes storage requirements for a container.
type Storage struct {
	Temp    map[string]int    `json:"tmp"`     // container path: max alloc megabytes (-1 for unlimited)
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
	Startup  int `json:"startup"`
	Shutdown int `json:"shutdown"`
}

// Valid performs a validation check, to ensure invalid structures may be
// detected as early as possible.
func (g Grace) Valid() error {
	var errs []string
	if g.Startup <= 0 || g.Startup > 30 {
		errs = append(errs, fmt.Sprintf("startup (%d) must be between 1 and 30", g.Startup))
	}
	if g.Shutdown <= 0 || g.Shutdown > 30 {
		errs = append(errs, fmt.Sprintf("shutdown (%d) must be between 1 and 30", g.Shutdown))
	}
	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}
	return nil
}

// HostResources are returned by agents and reflect their current state.
type HostResources struct {
	Memory  TotalReserved `json:"mem"`     // MB
	CPUs    TotalReserved `json:"cpus"`    // whole CPUs
	Storage TotalReserved `json:"storage"` // Bytes
	Volumes []string      `json:"volumes"`
}

// TotalReserved encodes the total scalar amount of an arbitrary resource
// (total) and the amount of it that's currently in-use (reserved).
type TotalReserved struct {
	Total    float64 `json:"total"`
	Reserved float64 `json:"reserved"`
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
	ID     string          `json:"container_id"`
	Status ContainerStatus `json:"status"`
	Config ContainerConfig `json:"config"`
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
	ContainerStatusRunning = "running"

	// ContainerStatusFailed indicates the container has exited with a nonzero
	// return code. In most cases, this is a very short-lived state, as the
	// agent will restart the container.
	ContainerStatusFailed = "failed"

	// ContainerStatusFinished indicates the container has exited successfully
	// with a zero return code. In most cases, this will be a long-lived
	// state, as the agent will not restart the container. (We should probably
	// think about if and how to reap finished containers.)
	ContainerStatusFinished = "finished"

	// ContainerStatusDeleted is a special meta-state used only in event
	// signaling. It's sent to event stream subscribers when a container is
	// successfully deleted. It should never be stored, only part of an event.
	ContainerStatusDeleted = "deleted"
)

// Heartbeat is information about a running container. It's sent from the
// container process to the agent via HTTP POST. The agent replies with
// HeartbeatReply.
//
// UP and DOWN are sent when the container has successfully reached those
// states. Once a container successfully sends a heartbeat with status DOWN,
// it won't send any more heartbeats (unless restarted).
type Heartbeat struct {
	Status    string    `json:"status"` // UP, DOWN
	Err       string    `json:"err,omitempty"`
	Timestamp time.Time `json:"timestamp"`

	ContainerProcessStatus `json:"container_process_status"`
}

// HeartbeatReply is information about the desired state of a running
// container.  It's sent from the agent to the container process as the
// response to a heartbeat POST.
//
// UP and DOWN are sent as part of the normal container lifecycle. FORCEDOWN
// is sent when the container hasn't transitioned from UP to DOWN within the
// configured shutdown grace period.
type HeartbeatReply struct {
	Want string `json:"want"` // UP, DOWN, FORCEDOWN
	Err  string `json:"err,omitempty"`
}

// ContainerProcessStatus contains detailed information about the state of the
// operating system process for a given container. All fields except
// ContainerMetrics reflect the current instance state.
type ContainerProcessStatus struct {
	Up bool `json:"up"`

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

	*ContainerMetrics `json:"container_metrics"`
}

// ContainerMetrics contains detailed historical information about a unique
// container. ContainerMetrics are tracked across restarts.
type ContainerMetrics struct {
	Restarts    uint64 `json:"restarts"`     // counter of restarts
	OOMs        uint64 `json:"ooms"`         // counter of ooms
	CPUTime     uint64 `json:"cpu_time"`     // total counter of cpu time
	MemoryUsage uint64 `json:"memory_usage"` // memory usage in bytes
	MemoryLimit uint64 `json:"memory_limit"` // memory limit in bytes
}
