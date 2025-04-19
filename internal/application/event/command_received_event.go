package event

import (
	"reflect"
	"time"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/core/event"
)

const EventTypeCommandReceived event.EventType = "command_received"

func NewCommandReceivedEvent(cmd command.Command) event.Event {
	return event.Event{
		Message:   "Command received: " + reflect.TypeOf(cmd).String(),
		Type:      EventTypeCommandReceived,
		Timestamp: time.Now(),
	}
}
