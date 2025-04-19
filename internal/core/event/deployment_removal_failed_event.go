package event

import (
	"fmt"
	"time"
)

const EventTypeDeploymentRemovalFailed EventType = "deployment_removal_failed"

func NewDeploymentRemovalFailedEvent(name string, err error) Event {
	return Event{
		Message:   fmt.Sprintf("Deployment %s removal failed: %v", name, err),
		Type:      EventTypeDeploymentRemovalFailed,
		Timestamp: time.Now(),
	}
}
