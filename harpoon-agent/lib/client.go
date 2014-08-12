package agent

const (
	// APIVersionPrefix identifies the version of the agent API served.
	APIVersionPrefix = "/api/v0"

	// APIListContainersPath conforms to the agent API spec.
	APIListContainersPath = "/containers"

	// APICreateContainerPath conforms to the agent API spec.
	APICreateContainerPath = "/containers/:id"

	// APIGetContainerPath conforms to the agent API spec.
	APIGetContainerPath = "/containers/:id"

	// APIDestroyContainerPath conforms to the agent API spec.
	APIDestroyContainerPath = "/containers/:id"

	// APIStartContainerPath conforms to the agent API spec.
	APIStartContainerPath = "/containers/:id/start"

	// APIStopContainerPath conforms to the agent API spec.
	APIStopContainerPath = "/containers/:id/stop"

	// APIGetContainerLogPath conforms to the agent API spec.
	APIGetContainerLogPath = "/containers/:id/log"

	// APIGetResourcesPath conforms to the agent API spec.
	APIGetResourcesPath = "/resources"
)
