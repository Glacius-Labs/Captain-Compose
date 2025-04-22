package deployment

import (
	"time"

	"github.com/glacius-labs/captain-compose/internal/core/event"
)

const TypeRemoved event.Type = "deployment_removed"

type RemovedEvent struct {
	DeploymentName string
	CreatedAt      time.Time
}

func NewRemovedEvent(deploymentName string) *RemovedEvent {
	return &RemovedEvent{
		DeploymentName: deploymentName,
		CreatedAt:      time.Now(),
	}
}

func (e *RemovedEvent) Type() event.Type {
	return TypeRemoved
}

func (e *RemovedEvent) Timestamp() time.Time {
	return e.CreatedAt
}
