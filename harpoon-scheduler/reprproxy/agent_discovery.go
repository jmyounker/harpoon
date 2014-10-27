package reprproxy

// AgentDiscovery allows components to find out about the remote agents
// available in a scheduling domain.
type AgentDiscovery interface {
	Subscribe(chan<- []string)
	Unsubscribe(chan<- []string)
}

// StaticAgentDiscovery encodes a fixed set of endpoints.
type StaticAgentDiscovery []string

// Subscribe satisfies the AgentDiscovery interface.
func (d StaticAgentDiscovery) Subscribe(c chan<- []string) { go func() { c <- d }() }

// Unsubscribe satisfies the AgentDiscovery interface.
func (d StaticAgentDiscovery) Unsubscribe(chan<- []string) { return }
