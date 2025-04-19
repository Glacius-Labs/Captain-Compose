package event

import (
	"fmt"
	"time"
)

const EventTypeDeploymentCreated EventType = "deployment_created"

func NewDeploymentCreatedEvent(name string) Event {
	return Event{
		Message:   fmt.Sprintf("Deployment %s created", name),
		Type:      EventTypeDeploymentCreated,
		Timestamp: time.Now(),
	}
}
