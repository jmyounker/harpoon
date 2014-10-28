package algo_test

import (
	"reflect"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-scheduler/algo"
)

func TestSort(t *testing.T) {
	var (
		endpoint2containers = map[string]int{
			"first.net":  1,
			"second.net": 2,
			"third.net":  3,
			"fourth.net": 4,
		}

		strategy = algo.LeastUsedSort(endpoint2containers)
		agents   = []string{"third.net", "first.net", "second.net", "fourth.net"}
	)

	strategy.Sort(agents)

	want := []string{"first.net", "second.net", "third.net", "fourth.net"}
	if !reflect.DeepEqual(want, agents) {
		t.Errorf("want %v, have %v", want, agents)
	}
}
