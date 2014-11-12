package main

import "github.com/soundcloud/harpoon/harpoon-agent/lib"

type containers map[string]agent.ContainerInstance

type agentAddr struct {
	agent.Agent
	addr string
}

type cluster []agentAddr

func (c cluster) Resources() (map[string]agent.HostResources, error) {
	results := map[string]agent.HostResources{}

	for _, a := range c {
		resources, err := a.Resources()
		if err != nil {
			return nil, err
		}

		results[a.addr] = resources
	}

	return results, nil
}

func (c cluster) Containers() (map[string]containers, error) {
	results := map[string]containers{}

	for _, a := range c {
		containers, err := a.Containers()
		if err != nil {
			return nil, err
		}

		results[a.addr] = containers
	}

	return results, nil
}

func (c cluster) Get(id string) (agent.ContainerInstance, error) {
	var (
		container agent.ContainerInstance
		err       error
	)

	for _, a := range c {
		container, err = a.Get(id)
		if err != nil {
			if err == agent.ErrContainerNotExist {
				continue
			}

			return agent.ContainerInstance{}, err
		}

		if container.ID == id {
			break
		}
	}

	if container.ID != id {
		return agent.ContainerInstance{}, agent.ErrContainerNotExist
	}

	return container, nil
}

func (c cluster) Stop(id string) error {
	for _, a := range c {
		if err := a.Stop(id); err != nil {
			if err == agent.ErrContainerNotExist {
				continue
			}

			return err
		}

		return nil
	}

	return agent.ErrContainerNotExist
}

func (c cluster) Destroy(id string) error {
	for _, a := range c {
		if err := a.Destroy(id); err != nil {
			if err == agent.ErrContainerNotExist {
				continue
			}

			return err
		}

		return nil
	}

	return agent.ErrContainerNotExist
}

func (c cluster) Start(id string) error {
	for _, a := range c {
		if err := a.Start(id); err != nil {
			if err == agent.ErrContainerNotExist {
				continue
			}

			return err
		}

		return nil
	}

	return agent.ErrContainerNotExist
}

func (c cluster) Log(id string, history int) (<-chan string, agent.Stopper, error) {
	for _, a := range c {
		lines, stopper, err := a.Log(id, history)
		if err != nil {
			if err == agent.ErrContainerNotExist {
				continue
			}

			return nil, nil, err
		}

		return lines, stopper, nil
	}

	return nil, nil, agent.ErrContainerNotExist
}
