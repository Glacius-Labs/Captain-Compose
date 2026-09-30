package mqtt

import (
	"encoding/json"
	"time"
)

const (
	TypeCreate = "create"
	TypeRemove = "remove"
)

type Envelope struct {
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	Version   int             `json:"version,omitempty"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
	Data      json.RawMessage `json:"data"`
}
