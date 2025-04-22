package deployment

import (
	"time"
)

const TypeRemoved = "deployment_removed"

type RemovedEvent struct {
	DeploymentName string
	CreatedAt      time.Time
}

func NewRemovedEvent(deploymentName string) *RemovedEvent {
	return &RemovedEvent{
		DeploymentName: deploymentName,
		CreatedAt:      time.Now(),
	}
}

func (e *RemovedEvent) Type() string {
	return TypeRemoved
}

func (e *RemovedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
