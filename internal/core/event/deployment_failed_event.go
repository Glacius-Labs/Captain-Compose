package event

import (
	"time"
)

const EventTypeDeploymentFailed EventType = "deployment_failed"

type DeploymentFailedEvent struct {
	DeploymentName string
	Error          string
	CreatedAt      time.Time
}

func NewDeploymentFailedEvent(deploymentName string, err error) *DeploymentFailedEvent {
	return &DeploymentFailedEvent{
		DeploymentName: deploymentName,
		Error:          err.Error(),
		CreatedAt:      time.Now(),
	}
}

func (e *DeploymentFailedEvent) Type() EventType {
	return EventTypeDeploymentFailed
}

func (e *DeploymentFailedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
