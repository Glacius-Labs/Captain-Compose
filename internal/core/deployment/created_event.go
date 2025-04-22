package deployment

import (
	"time"
)

const TypeCreated string = "deployment_created"

type CreatedEvent struct {
	DeploymentName string
	CreatedAt      time.Time
}

func NewCreatedEvent(deploymentName string) *CreatedEvent {
	return &CreatedEvent{
		DeploymentName: deploymentName,
		CreatedAt:      time.Now(),
	}
}

func (e *CreatedEvent) Type() string {
	return TypeCreated
}

func (e *CreatedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
