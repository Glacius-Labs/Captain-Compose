package event

import (
	"fmt"
	"time"
)

const EventTypeDeploymentFailed EventType = "deployment_failed"

func NewDeploymentFailedEvent(name string, err error) Event {
	return Event{
		Message:   fmt.Sprintf("Deployment %s failed: %v", name, err),
		Type:      EventTypeDeploymentFailed,
		Timestamp: time.Now(),
	}
}
