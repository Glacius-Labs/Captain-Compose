package deployment

import (
	"time"

	"github.com/google/uuid"
)

const TypeCreationFailed = "deployment_creation_failed"

type CreationFailedEvent struct {
	ID             uuid.UUID
	DeploymentName string
	Payload        []byte
	Error          string
	CreatedAt      time.Time
}

func NewCreationFailedEvent(deploymentName string, payload []byte, err error) *CreationFailedEvent {
	return &CreationFailedEvent{
		ID:             uuid.New(),
		DeploymentName: deploymentName,
		Payload:        payload,
		Error:          err.Error(),
		CreatedAt:      time.Now(),
	}
}

func (e *CreationFailedEvent) Identifier() uuid.UUID {
	return e.ID
}

func (e *CreationFailedEvent) Type() string {
	return TypeCreationFailed
}

func (e *CreationFailedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
