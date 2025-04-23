package event

import (
	"time"

	"github.com/google/uuid"
)

type Event interface {
	Identifier() uuid.UUID
	Type() string
	Timestamp() time.Time
}
