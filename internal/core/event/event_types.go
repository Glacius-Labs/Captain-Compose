package event

type EventType string

const (
	EventTypeDeploymentCreated       EventType = "deployment_created"
	EventTypeDeploymentFailed        EventType = "deployment_failed"
	EventTypeDeploymentRemoved       EventType = "deployment_removed"
	EventTypeDeploymentRemovalFailed EventType = "deployment_removal_failed"
)
