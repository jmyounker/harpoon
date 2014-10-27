package reprproxy_test

import (
	"reflect"
	"testing"

	"github.com/soundcloud/harpoon/harpoon-scheduler/reprproxy"
)

func TestStaticAgentDiscovery(t *testing.T) {
	const (
		endpoint1 = "http://computers.berlin:31337"
		endpoint2 = "http://kraftwerk.info:8080"
	)

	var (
		discovery = reprproxy.StaticAgentDiscovery([]string{endpoint1, endpoint2})
		updatec   = make(chan []string)
		requestc  = make(chan []string)
	)

	discovery.Subscribe(updatec)
	defer close(updatec)
	defer discovery.Unsubscribe(updatec)

	go func() {
		endpoints, ok := <-updatec
		if !ok {
			return
		}

		for {
			select {
			case requestc <- endpoints:
			case endpoints, ok = <-updatec:
				if !ok {
					return
				}
			}
		}
	}()

	if want, have := []string{endpoint1, endpoint2}, <-requestc; !reflect.DeepEqual(want, have) {
		t.Errorf("want %v, have %v", want, have)
	}
}
