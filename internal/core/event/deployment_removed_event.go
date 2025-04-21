package event

import (
	"time"
)

const EventTypeDeploymentRemoved EventType = "deployment_removed"

type DeploymentRemovedEvent struct {
	DeploymentName string
	CreatedAt      time.Time
}

func NewDeploymentRemovedEvent(deploymentName string) *DeploymentRemovedEvent {
	return &DeploymentRemovedEvent{
		DeploymentName: deploymentName,
		CreatedAt:      time.Now(),
	}
}

func (e *DeploymentRemovedEvent) Type() EventType {
	return EventTypeDeploymentRemoved
}

func (e *DeploymentRemovedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
