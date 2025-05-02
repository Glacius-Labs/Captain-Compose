package message

import (
	"encoding/json"
	"fmt"
)

type Envelope struct {
	Type    Type            `json:"type"`
	Version string          `json:"version"`
	Payload json.RawMessage `json:"payload"`
}

func DecodeEnvelope(data []byte) (*Envelope, error) {
	var env Envelope
	if err := json.Unmarshal(data, &env); err != nil {
		return nil, fmt.Errorf("invalid envelope format: %w", err)
	}

	return &env, nil
}
