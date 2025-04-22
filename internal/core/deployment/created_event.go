package deployment

import (
	"time"

	"github.com/glacius-labs/captain-compose/internal/core/event"
)

const TypeCreated event.Type = "deployment_created"

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

func (e *CreatedEvent) Type() event.Type {
	return TypeCreated
}

func (e *CreatedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
