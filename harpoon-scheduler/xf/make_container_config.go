package xf

import "fmt"

func makeContainerID(jobConfigHash string, i int) string {
	return fmt.Sprintf("%s-%d", jobConfigHash, i)
}
