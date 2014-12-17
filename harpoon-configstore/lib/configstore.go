package configstore

import (
	"crypto/md5"
	"encoding/json"
	"fmt"
	"strings"

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
	Scale int `json:"scale"`
	agent.ContainerConfig
}

// Valid performs a validation check, to ensure invalid structures may be
// detected as early as possible.
func (c JobConfig) Valid() error {
	var errs []string

	if c.Scale <= 0 || c.Scale > 1000 {
		errs = append(errs, fmt.Sprintf("scale of %d is invalid", c.Scale))
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
