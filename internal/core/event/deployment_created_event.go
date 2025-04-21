package event

import (
	"time"

	"github.com/glacius-labs/captain-compose/internal/core/model"
)

const EventTypeDeploymentCreated EventType = "deployment_created"

type DeploymentCreatedEvent struct {
	Deployment model.Deployment
	CreatedAt  time.Time
}

func NewDeploymentCreatedEvent(deployment model.Deployment) *DeploymentCreatedEvent {
	return &DeploymentCreatedEvent{
		Deployment: deployment,
		CreatedAt:  time.Now(),
	}
}

func (e *DeploymentCreatedEvent) Type() EventType {
	return EventTypeDeploymentCreated
}

func (e *DeploymentCreatedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
