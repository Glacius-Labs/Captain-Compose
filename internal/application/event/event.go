package event

import (
	"time"
)

type Event interface {
	Type() string
	Timestamp() time.Time
}
