package deployment

import (
	"time"
)

const TypeCreationFailed = "deployment_creation_failed"

type CreationFailedEvent struct {
	DeploymentName string
	Error          string
	CreatedAt      time.Time
}

func NewCreationFailedEvent(deploymentName string, err error) *CreationFailedEvent {
	return &CreationFailedEvent{
		DeploymentName: deploymentName,
		Error:          err.Error(),
		CreatedAt:      time.Now(),
	}
}

func (e *CreationFailedEvent) Type() string {
	return TypeCreationFailed
}

func (e *CreationFailedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
