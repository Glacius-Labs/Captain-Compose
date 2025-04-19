package event

import (
	"fmt"
	"time"
)

const EventTypeDeploymentRemoved EventType = "deployment_removed"

func NewDeploymentRemovedEvent(name string) Event {
	return Event{
		Message:   fmt.Sprintf("Deployment %s removed", name),
		Type:      EventTypeDeploymentRemoved,
		Timestamp: time.Now(),
	}
}
