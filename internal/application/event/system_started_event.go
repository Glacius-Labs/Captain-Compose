package event

import (
	"time"

	"github.com/glacius-labs/captain-compose/internal/core/event"
)

const EventTypeSystemStarted event.EventType = "system_started"

func NewSystemStartedEvent() event.Event {
	return event.Event{
		Message:   "System started",
		Type:      EventTypeSystemStarted,
		Timestamp: time.Now(),
	}
}
