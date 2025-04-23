package deployment

import (
	"time"

	"github.com/google/uuid"
)

const TypeRemoved = "deployment_removed"

type RemovedEvent struct {
	ID             uuid.UUID
	DeploymentName string
	CreatedAt      time.Time
}

func NewRemovedEvent(deploymentName string) *RemovedEvent {
	return &RemovedEvent{
		ID:             uuid.New(),
		DeploymentName: deploymentName,
		CreatedAt:      time.Now(),
	}
}

func (e *RemovedEvent) Identifier() uuid.UUID {
	return e.ID
}

func (e *RemovedEvent) Type() string {
	return TypeRemoved
}

func (e *RemovedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
