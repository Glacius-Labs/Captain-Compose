package event

import (
	"encoding/json"
	"time"

	"github.com/glacius-labs/captain-compose/internal/application/event"
	"github.com/google/uuid"
)

const Version string = "v1"

type Envelope struct {
	ID        uuid.UUID       `json:"id"`
	Type      string          `json:"type"`
	Timestamp time.Time       `json:"timestamp"`
	Version   string          `json:"version"`
	Payload   json.RawMessage `json:"payload"`
}

func Pack(e event.Event) (*Envelope, error) {
	payload, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}

	return &Envelope{
		ID:        e.Identifier(),
		Type:      e.Type(),
		Timestamp: e.Timestamp(),
		Version:   Version,
		Payload:   payload,
	}, nil
}

func MustPack(e event.Event) *Envelope {
	env, err := Pack(e)
	if err != nil {
		panic("failed to pack event: " + err.Error())
	}
	return env
}
