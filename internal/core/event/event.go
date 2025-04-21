package event

import (
	"time"
)

type EventType string

type Event interface {
	Type() EventType
	Timestamp() time.Time
}
