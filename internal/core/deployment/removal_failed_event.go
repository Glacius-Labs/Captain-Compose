package deployment

import (
	"time"
)

const TypeRemovalFailed = "deployment_removal_failed"

type RemovalFailedEvent struct {
	DeploymentName string
	Error          string
	CreatedAt      time.Time
}

func NewRemovalFailedEvent(deploymentName string, err error) *RemovalFailedEvent {
	return &RemovalFailedEvent{
		DeploymentName: deploymentName,
		Error:          err.Error(),
		CreatedAt:      time.Now(),
	}
}

func (e *RemovalFailedEvent) Type() string {
	return TypeRemovalFailed
}

func (e *RemovalFailedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
