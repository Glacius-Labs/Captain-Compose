package deployment

import (
	"time"

	"github.com/google/uuid"
)

const EventTypeRemovalFailed = "deployment.removal_failed"

type RemovalFailedEvent struct {
	ID             uuid.UUID
	DeploymentName string
	Error          string
	CreatedAt      time.Time
}

func NewRemovalFailedEvent(deploymentName string, err error) *RemovalFailedEvent {
	return &RemovalFailedEvent{
		ID:             uuid.New(),
		DeploymentName: deploymentName,
		Error:          err.Error(),
		CreatedAt:      time.Now(),
	}
}

func (e *RemovalFailedEvent) Identifier() uuid.UUID {
	return e.ID
}

func (e *RemovalFailedEvent) Type() string {
	return EventTypeRemovalFailed
}

func (e *RemovalFailedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
