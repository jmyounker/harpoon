package algo

import (
	"reflect"
	"testing"
)

func TestSort(t *testing.T) {
	var (
		e2c = map[string]int{
			"first.net":  1,
			"second.net": 2,
			"third.net":  3,
			"fourth.net": 4,
		}

		strategy = leastUsed{e2c: e2c}
		agents   = []string{"third.net", "first.net", "second.net", "fourth.net"}
	)

	strategy.sort(agents)

	want := []string{"first.net", "second.net", "third.net", "fourth.net"}
	if !reflect.DeepEqual(want, agents) {
		t.Errorf("want %v, have %v", want, agents)
	}
}
