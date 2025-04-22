package event

import (
	"time"
)

type Type string

type Event interface {
	Type() Type
	Timestamp() time.Time
}
