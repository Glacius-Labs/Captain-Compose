package event

import (
	"fmt"
	"time"
)

type Event struct {
	Message   string
	Type      EventType
	Timestamp time.Time
}

func NewDeploymentCreatedEvent(name string) Event {
	return Event{
		Message:   fmt.Sprintf("Deployment %s created", name),
		Type:      EventTypeDeploymentCreated,
		Timestamp: time.Now(),
	}
}

func NewDeploymentFailedEvent(name string, err error) Event {
	return Event{
		Message:   fmt.Sprintf("Deployment %s failed: %v", name, err),
		Type:      EventTypeDeploymentFailed,
		Timestamp: time.Now(),
	}
}

func NewDeploymentRemovedEvent(name string) Event {
	return Event{
		Message:   fmt.Sprintf("Deployment %s removed", name),
		Type:      EventTypeDeploymentRemoved,
		Timestamp: time.Now(),
	}
}

func NewDeploymentRemovalFailedEvent(name string, err error) Event {
	return Event{
		Message:   fmt.Sprintf("Deployment %s removal failed: %v", name, err),
		Type:      EventTypeDeploymentRemovalFailed,
		Timestamp: time.Now(),
	}
}
