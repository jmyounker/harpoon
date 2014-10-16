package main

import (
	"fmt"
	"strings"
)

// telemetryLabels is a prometheus label set provided as flags.
type telemetryLabels map[string]string

func (labels telemetryLabels) String() string { return "" }

func (labels telemetryLabels) Set(value string) error {
	parts := strings.SplitN(value, "=", 2)

	if len(parts) != 2 {
		return fmt.Errorf("invalid label %q, should be in the format of K=V", value)
	}

	labels[parts[0]] = parts[1]
	return nil
}
