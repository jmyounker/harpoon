package scheduler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"

	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

type client struct{ url.URL }

var (
	pathSchedule   = "schedule"
	pathUnschedule = "unschedule"
	pathState      = "status"

	ErrConfigNotValid     = fmt.Errorf("config not valid")
	ErrConfigNotScheduled = fmt.Errorf("config not scheduled")
)

type scheduler interface {
	Schedule(configstore.JobConfig) (map[string]configstore.JobConfig, error)
	Unschedule(configstore.JobConfig) (map[string]configstore.JobConfig, error)
}

func NewClient(endpoint string) (scheduler, error) {
	u, err := url.Parse(endpoint)
	if err != nil {
		return &client{}, err
	}

	return &client{URL: *u}, nil
}

func (cl *client) Schedule(cfg configstore.JobConfig) (map[string]configstore.JobConfig, error) {
	cl.URL.Path = pathSchedule
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(cfg); err != nil {
		return map[string]configstore.JobConfig{}, fmt.Errorf("problem encoding container config (%s)", err)
	}

	req, err := http.NewRequest("POST", cl.URL.String(), &body)
	if err != nil {
		return map[string]configstore.JobConfig{}, fmt.Errorf("problem constructing HTTP request (%s)", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]configstore.JobConfig{}, fmt.Errorf("agent unavailable (%s)", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusAccepted:
		var state map[string]configstore.JobConfig
		if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
			return map[string]configstore.JobConfig{}, fmt.Errorf("invalid agent response (%s)", err)
		}
		return state, nil

	default:
		buf, _ := ioutil.ReadAll(resp.Body)
		return map[string]configstore.JobConfig{}, fmt.Errorf("HTTP %d (%s)", resp.StatusCode, bytes.TrimSpace(buf))
	}

}

func (cl *client) Unschedule(cfg configstore.JobConfig) (map[string]configstore.JobConfig, error) {
	cl.URL.Path = pathUnschedule
	var body bytes.Buffer
	if err := json.NewEncoder(&body).Encode(cfg); err != nil {
		return map[string]configstore.JobConfig{}, fmt.Errorf("problem encoding container config (%s)", err)
	}

	req, err := http.NewRequest("POST", cl.URL.String(), &body)
	if err != nil {
		return map[string]configstore.JobConfig{}, fmt.Errorf("problem constructing HTTP request (%s)", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return map[string]configstore.JobConfig{}, fmt.Errorf("agent unavailable (%s)", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusAccepted:
		var state map[string]configstore.JobConfig
		if err := json.NewDecoder(resp.Body).Decode(&state); err != nil {
			return map[string]configstore.JobConfig{}, fmt.Errorf("invalid agent response (%s)", err)
		}
		return state, nil

	default:
		buf, _ := ioutil.ReadAll(resp.Body)
		return map[string]configstore.JobConfig{}, fmt.Errorf("HTTP %d (%s)", resp.StatusCode, bytes.TrimSpace(buf))
	}
}
