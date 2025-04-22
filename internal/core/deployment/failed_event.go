package deployment

import (
	"time"

	"github.com/glacius-labs/captain-compose/internal/core/event"
)

const TypeFailed event.Type = "deployment_failed"

type FailedEvent struct {
	DeploymentName string
	Error          string
	CreatedAt      time.Time
}

func NewFailedEvent(deploymentName string, err error) *FailedEvent {
	return &FailedEvent{
		DeploymentName: deploymentName,
		Error:          err.Error(),
		CreatedAt:      time.Now(),
	}
}

func (e *FailedEvent) Type() event.Type {
	return TypeFailed
}

func (e *FailedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
