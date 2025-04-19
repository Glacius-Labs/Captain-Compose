package event

import (
	"time"
)

type EventType string

type Event struct {
	Message   string
	Type      EventType
	Timestamp time.Time
}
