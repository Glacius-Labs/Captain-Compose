package event

import "time"

type Event struct {
	Message   string
	Type      EventType
	Timestamp time.Time
}
