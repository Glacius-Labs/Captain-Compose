package deployment

import (
	"time"

	"github.com/google/uuid"
)

const EventTypeCreated string = "deployment.created"

type CreatedEvent struct {
	ID             uuid.UUID
	DeploymentName string
	Payload        []byte
	CreatedAt      time.Time
}

func NewCreatedEvent(deploymentName string, payload []byte) *CreatedEvent {
	return &CreatedEvent{
		ID:             uuid.New(),
		DeploymentName: deploymentName,
		Payload:        payload,
		CreatedAt:      time.Now(),
	}
}

func (e *CreatedEvent) Identifier() uuid.UUID {
	return e.ID
}

func (e *CreatedEvent) Type() string {
	return EventTypeCreated
}

func (e *CreatedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
