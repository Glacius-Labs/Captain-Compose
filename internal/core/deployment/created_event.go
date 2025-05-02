package deployment

import (
	"time"

	"github.com/google/uuid"
)

const EvetTypeCreated string = "deployment.created"

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
	return EvetTypeCreated
}

func (e *CreatedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
