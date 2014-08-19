package agent

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/bernerdschaefer/eventsource"
)

const (
	// APIVersionPrefix identifies the version of the API that this code
	// serves and expects. Non-backwards-compatible API changes should
	// increment the version.

	// APIVersionPrefix identifies the version of the agent API served.
	APIVersionPrefix = "/api/v0"

	// APIListContainersPath conforms to the agent API spec.
	APIListContainersPath = "/containers"

	// APICreateContainerPath conforms to the agent API spec.
	APICreateContainerPath = "/containers/:id"

	// APIGetContainerPath conforms to the agent API spec.
	APIGetContainerPath = "/containers/:id"

	// APIDestroyContainerPath conforms to the agent API spec.
	APIDestroyContainerPath = "/containers/:id"

	// APIStartContainerPath conforms to the agent API spec.
	APIStartContainerPath = "/containers/:id/start"

	// APIStopContainerPath conforms to the agent API spec.
	APIStopContainerPath = "/containers/:id/stop"

	// APIGetContainerLogPath conforms to the agent API spec.
	APIGetContainerLogPath = "/containers/:id/log"

	// APIGetResourcesPath conforms to the agent API spec.
	APIGetResourcesPath = "/resources"
)

var (
	// ErrContainerNotExist is returned when clients try to interact with a
	// container that doesn't exist on the agent.
	ErrContainerNotExist = errors.New("container doesn't exist")

	// ErrContainerAlreadyExists is returned when clients try to Put a
	// container that already exists on the agent.
	ErrContainerAlreadyExists = errors.New("container already exists")

	// ErrContainerAlreadyRunning is returned when clients try to Start a
	// container that's already in ContainerStatusRunning.
	ErrContainerAlreadyRunning = errors.New("container already running")

	// ErrContainerAlreadyStopped is returned when clients try to Stop a
	// container that's already in ContainerStatusFinished.
	ErrContainerAlreadyStopped = errors.New("container already stopped")
)

type client struct{ url.URL }

var _ Agent = client{}

// NewClient produces an Agent that proxies requests to the remote agent at
// endpoint.
func NewClient(endpoint string) (Agent, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return client{}, err
	}

	return client{URL: *u}, nil
}

// Containers implements the Agent interface.
func (c client) Containers() (map[string]ContainerInstance, error) {
	c.URL.Path = APIVersionPrefix + APIListContainersPath

	req, err := http.NewRequest("GET", c.URL.String(), nil)
	if err != nil {
		return map[string]ContainerInstance{}, fmt.Errorf("problem constructing HTTP request (%s)", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]ContainerInstance{}, fmt.Errorf("agent unavailable (%s)", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var instances map[string]ContainerInstance
		if err := json.NewDecoder(resp.Body).Decode(&instances); err != nil {
			return map[string]ContainerInstance{}, fmt.Errorf("invalid agent response (%s)", err)
		}
		return instances, nil

	default:
		buf, _ := ioutil.ReadAll(resp.Body)
		return map[string]ContainerInstance{}, fmt.Errorf("HTTP %d (%s)", resp.StatusCode, bytes.TrimSpace(buf))
	}
}

// Events implements the Agent interface.
func (c client) Events() (<-chan map[string]ContainerInstance, Stopper, error) {
	c.URL.Path = APIVersionPrefix + APIListContainersPath

	req, err := http.NewRequest("GET", c.URL.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("problem constructing HTTP request (%s)", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	var (
		statec = make(chan map[string]ContainerInstance)
		stopc  = make(chan struct{})
		es     = eventsource.New(req, 1*time.Second)
	)

	go func() {
		<-stopc
		es.Close()
	}()

	go func() {
		defer close(statec)
		for {
			// We need the EventSource to tell us when the connection is
			// interrupted, rather than transparently handling it.

			event, err := es.Read()
			if err != nil {
				log.Printf("%s: %s", c.URL.String(), err)
				return
			}

			var state map[string]ContainerInstance
			if err := json.Unmarshal(event.Data, &state); err != nil {
				log.Printf("%s: %s", c.URL.String(), err)
				continue
			}

			statec <- state
		}
	}()

	return statec, stopperChan(stopc), nil
}

type containerEvent interface {
	eventName() string
}

// Resources implements the Agent interface.
func (c client) Resources() (HostResources, error) {
	c.URL.Path = APIVersionPrefix + APIGetResourcesPath

	req, err := http.NewRequest("GET", c.URL.String(), nil)
	if err != nil {
		return HostResources{}, fmt.Errorf("problem constructing HTTP request (%s)", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return HostResources{}, fmt.Errorf("agent unavailable (%s)", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var resources HostResources
		if err := json.NewDecoder(resp.Body).Decode(&resources); err != nil {
			return HostResources{}, fmt.Errorf("invalid agent response (%s)", err)
		}
		return resources, nil

	default:
		buf, _ := ioutil.ReadAll(resp.Body)
		return HostResources{}, fmt.Errorf("HTTP %d (%s)", resp.StatusCode, bytes.TrimSpace(buf))
	}
}

// Put implements the Agent interface.
func (c client) Put(id string, cfg ContainerConfig) error {
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(cfg); err != nil {
		return fmt.Errorf("problem encoding container config (%s)", err)
	}

	c.URL.Path = APIVersionPrefix + APICreateContainerPath
	c.URL.Path = strings.Replace(c.URL.Path, ":id", id, 1)

	req, err := http.NewRequest("PUT", c.URL.String(), &body)
	if err != nil {
		return fmt.Errorf("problem constructing HTTP request (%s)", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("agent unavailable (%s)", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusCreated:
		return nil

	case http.StatusConflict:
		return ErrContainerAlreadyExists

	default:
		buf, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d (%s)", resp.StatusCode, bytes.TrimSpace(buf))
	}
}

// Get implements the Agent interface.
func (c client) Get(id string) (ContainerInstance, error) {
	c.URL.Path = APIVersionPrefix + APIGetContainerPath
	c.URL.Path = strings.Replace(c.URL.Path, ":id", id, 1)

	req, err := http.NewRequest("GET", c.URL.String(), nil)
	if err != nil {
		return ContainerInstance{}, fmt.Errorf("problem constructing HTTP request (%s)", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return ContainerInstance{}, fmt.Errorf("agent unavailable (%s)", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var state ContainerInstance
		if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
			return ContainerInstance{}, fmt.Errorf("invalid agent response (%s)", err)
		}
		return state, nil

	case http.StatusNotFound:
		return ContainerInstance{}, ErrContainerNotExist

	default:
		buf, _ := ioutil.ReadAll(resp.Body)
		return ContainerInstance{}, fmt.Errorf("HTTP %d (%s)", resp.StatusCode, bytes.TrimSpace(buf))
	}
}

// Delete implements the Agent interface.
func (c client) Delete(id string) error {
	c.URL.Path = APIVersionPrefix + APIDestroyContainerPath
	c.URL.Path = strings.Replace(c.URL.Path, ":id", id, 1)

	req, err := http.NewRequest("DELETE", c.URL.String(), nil)
	if err != nil {
		return fmt.Errorf("problem constructing HTTP request (%s)", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("agent unavailable (%s)", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil

	case http.StatusNotFound:
		return ErrContainerNotExist

	default:
		buf, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d (%s)", resp.StatusCode, bytes.TrimSpace(buf))
	}
}

// Start implements the Agent interface.
func (c client) Start(id string) error {
	c.URL.Path = APIVersionPrefix + APIStartContainerPath
	c.URL.Path = strings.Replace(c.URL.Path, ":id", id, 1)

	req, err := http.NewRequest("POST", c.URL.String(), nil)
	if err != nil {
		return fmt.Errorf("problem constructing HTTP request (%s)", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("agent unavailable (%s)", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusAccepted:
		return nil

	case http.StatusNotFound:
		return ErrContainerNotExist

	case http.StatusConflict:
		return ErrContainerAlreadyRunning

	default:
		buf, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d (%s)", resp.StatusCode, bytes.TrimSpace(buf))
	}
}

// Stop implements the Agent interface.
func (c client) Stop(id string) error {
	c.URL.Path = APIVersionPrefix + APIStopContainerPath
	c.URL.Path = strings.Replace(c.URL.Path, ":id", id, 1)

	req, err := http.NewRequest("POST", c.URL.String(), nil)
	if err != nil {
		return fmt.Errorf("problem constructing HTTP request (%s)", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("agent unavailable (%s)", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusAccepted:
		return nil

	case http.StatusNotFound:
		return ErrContainerNotExist

	case http.StatusConflict:
		return ErrContainerAlreadyStopped

	default:
		buf, _ := ioutil.ReadAll(resp.Body)
		return fmt.Errorf("HTTP %d (%s)", resp.StatusCode, bytes.TrimSpace(buf))
	}
}

// Replace implements the Agent interface.
func (c client) Replace(newID, oldID string) error {
	return fmt.Errorf("replace is not implemented or used by the harpoon scheduler")
}

// Log implements the Agent interface.
func (c client) Log(id string, history int) (<-chan string, Stopper, error) {
	c.URL.Path = APIVersionPrefix + APIGetContainerLogPath
	c.URL.Path = strings.Replace(c.URL.Path, ":id", id, 1)
	c.URL.RawQuery = fmt.Sprintf("history=%d", history)
	req, err := http.NewRequest("GET", c.URL.String(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("problem constructing HTTP request (%s)", err)
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("agent unavailable (%s)", err)
	}
	// Because we're streaming, we close the body in a different way.

	switch resp.StatusCode {
	case http.StatusOK:
		c, stop := make(chan string), make(chan struct{})
		go func() {
			defer resp.Body.Close()
			defer close(c)

			rd := bufio.NewReader(resp.Body)
			for {
				line, err := rd.ReadString('\n')
				if err != nil {
					return
				}
				select {
				case c <- line:
				case <-stop:
					return
				}
			}
		}()
		return c, stopperChan(stop), nil

	case http.StatusNotFound:
		return nil, nil, ErrContainerNotExist

	default:
		defer resp.Body.Close()
		buf, _ := ioutil.ReadAll(resp.Body)
		return nil, nil, fmt.Errorf("HTTP %d (%s)", resp.StatusCode, bytes.TrimSpace(buf))
	}
}

type stopperChan chan struct{}

func (c stopperChan) Stop() { close(c) }
