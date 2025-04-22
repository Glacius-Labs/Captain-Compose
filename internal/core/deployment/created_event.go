package deployment

import (
	"time"
)

const TypeCreated string = "deployment_created"

type CreatedEvent struct {
	Deployment Deployment
	CreatedAt  time.Time
}

func NewCreatedEvent(deployment Deployment) *CreatedEvent {
	return &CreatedEvent{
		Deployment: deployment,
		CreatedAt:  time.Now(),
	}
}

func (e *CreatedEvent) Type() string {
	return TypeCreated
}

func (e *CreatedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
