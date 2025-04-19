package event

import (
	"time"

	"github.com/glacius-labs/captain-compose/internal/core/event"
)

const EventTypeSystemShutdown event.EventType = "system_shutdown"

func NewSystemShutdownEvent() event.Event {
	return event.Event{
		Message:   "System shutdown",
		Type:      EventTypeSystemShutdown,
		Timestamp: time.Now(),
	}
}
