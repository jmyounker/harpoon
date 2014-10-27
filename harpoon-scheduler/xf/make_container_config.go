package xf

import (
	"fmt"

	"github.com/soundcloud/harpoon/harpoon-configstore/lib"
)

func makeContainerID(config configstore.JobConfig, i int) string {
	return fmt.Sprintf("%s-%d", config.Hash(), i)
}
