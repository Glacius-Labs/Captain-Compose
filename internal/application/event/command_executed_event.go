package event

import (
	"reflect"
	"time"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/core/event"
)

const EventTypeCommandExecuted event.EventType = "command_executed"

func NewCommandExecutedEvent(cmd command.Command) event.Event {
	return event.Event{
		Message:   "Command executed: " + reflect.TypeOf(cmd).String(),
		Type:      EventTypeCommandExecuted,
		Timestamp: time.Now(),
	}
}
