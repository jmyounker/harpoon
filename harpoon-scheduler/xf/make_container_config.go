package xf

import "fmt"

// MakeContainerID produces the canonical container ID for a given job config
// hash and scale number (0-indexed).
func MakeContainerID(jobConfigHash string, i int) string {
	return fmt.Sprintf("%s-%d", jobConfigHash, i)
}
