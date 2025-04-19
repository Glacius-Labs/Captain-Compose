package event

import (
	"reflect"
	"time"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/core/event"
)

const EventTypeCommandFailed event.EventType = "command_failed"

func NewCommandFailedEvent(cmd command.Command, err error) event.Event {
	return event.Event{
		Message:   "Command failed for " + reflect.TypeOf(cmd).String() + " with error: " + err.Error(),
		Type:      EventTypeCommandFailed,
		Timestamp: time.Now(),
	}
}
