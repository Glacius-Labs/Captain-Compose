package event

import (
	"time"
)

const EventTypeDeploymentRemovalFailed EventType = "deployment_removal_failed"

type DeploymentRemovalFailedEvent struct {
	DeploymentName string
	Error          string
	CreatedAt      time.Time
}

func NewDeploymentRemovalFailedEvent(deploymentName string, err error) *DeploymentRemovalFailedEvent {
	return &DeploymentRemovalFailedEvent{
		DeploymentName: deploymentName,
		Error:          err.Error(),
		CreatedAt:      time.Now(),
	}
}

func (e *DeploymentRemovalFailedEvent) Type() EventType {
	return EventTypeDeploymentRemovalFailed
}

func (e *DeploymentRemovalFailedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
