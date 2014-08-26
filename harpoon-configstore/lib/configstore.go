package configstore

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/soundcloud/harpoon/harpoon-agent/lib"
)

// ConfigStore defines read and write behavior expected from a config store.
type ConfigStore interface {
	Get(ref string) (JobConfig, error)
	Put(JobConfig) (ref string, err error)
}

// JobConfig defines a configuration for a job, which is a collection of
// identical tasks. JobConfigs are declared by the user and stored in the
// config store. JobConfigs are maintained and persisted by the scheduler when
// they're scheduled.
type JobConfig struct {
	Job          string            `json:"job"`         // goku-activity, stream-api, dispatcher-web, etc.
	Environment  string            `json:"environment"` // dev, staging, prod
	Product      string            `json:"product"`     // search, stream, revdev, etc.
	Scale        int               `json:"scale"`
	Env          map[string]string `json:"env"`
	HealthChecks []HealthCheck     `json:"health_checks"`

	agent.ContainerConfig
}

// Valid performs a validation check, to ensure invalid structures may be
// detected as early as possible.
func (c JobConfig) Valid() error {
	var errs []string

	if c.Job == "" {
		errs = append(errs, `"job" not set`)
	}

	if c.Environment == "" {
		errs = append(errs, `"environment" not set`)
	}

	if c.Product == "" {
		errs = append(errs, `"product" not set`)
	}

	if c.Scale <= 0 || c.Scale > 1000 {
		errs = append(errs, fmt.Sprintf("scale of %d is invalid", c.Scale))
	}

	for i, healthCheck := range c.HealthChecks {
		if err := healthCheck.Valid(); err != nil {
			errs = append(errs, fmt.Sprintf("health check %d: %s", i, err))
		}
	}

	if err := c.ContainerConfig.Valid(); err != nil {
		errs = append(errs, err.Error())
	}

	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}

	return nil
}

// Hash produces a short and unique content-addressable string.
func (c JobConfig) Hash() string {
	h := md5.New()

	// JSON isn't a good encoding, because it's not stable.

	if err := json.NewEncoder(h).Encode(c); err != nil {
		panic(fmt.Sprintf("JobConfig Hash error: %s", err))
	}

	return fmt.Sprintf("%s-%s", c.Job, fmt.Sprintf("%x", h.Sum(nil))[:7])
}

// HealthCheck defines how a third party can determine if an instance of a
// given task is healthy. HealthChecks are defined and persisted in the config
// store, but executed by the agent or scheduler.
//
// HealthChecks are largely inspired by the Marathon definition.
// https://github.com/mesosphere/marathon/blob/master/REST.md
type HealthCheck struct {
	Protocol     string             `json:"protocol"` // HTTP, TCP
	Port         string             `json:"port"`     // from key of ports map in container config, i.e. env var name
	InitialDelay agent.JSONDuration `json:"initial_delay"`
	Timeout      agent.JSONDuration `json:"timeout"`
	Interval     agent.JSONDuration `json:"interval"`

	// Special parameters for HTTP health checks.
	HTTPPath                string `json:"http_path,omitempty"`                 // e.g. "/-/health"
	HTTPAcceptableResponses []int  `json:"http_acceptable_responses,omitempty"` // e.g. [200,201,301]
}

const (
	protocolHTTP = "HTTP"
	protocolTCP  = "TCP"

	maxInitialDelay = 30 * time.Second
	maxTimeout      = 3 * time.Second
	maxInterval     = 30 * time.Second
)

// Valid performs a validation check, to ensure invalid structures may be
// detected as early as possible.
func (c HealthCheck) Valid() error {
	var errs []string

	switch c.Protocol {
	case protocolHTTP, protocolTCP:
	default:
		errs = append(errs, fmt.Sprintf("invalid protocol %q", c.Protocol))
	}

	if c.InitialDelay.Duration > maxInitialDelay {
		errs = append(errs, fmt.Sprintf("initial delay (%s) too large (max %s)", c.InitialDelay, maxInitialDelay))
	}

	if c.Timeout.Duration > maxTimeout {
		errs = append(errs, fmt.Sprintf("timeout (%s) too large (max %s)", c.Timeout, maxTimeout))
	}

	if c.Interval.Duration > maxInterval {
		errs = append(errs, fmt.Sprintf("interval (%s) too large (max %s)", c.Interval, maxInterval))
	}

	if c.Protocol == protocolHTTP {
		if c.HTTPPath == "" {
			errs = append(errs, `protocol "HTTP" requires "http_path"`)
		}

		if len(c.HTTPAcceptableResponses) <= 0 {
			errs = append(errs, `protocol "HTTP" requires "http_acceptable_responses" (array of integers)`)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf(strings.Join(errs, "; "))
	}

	return nil
}
