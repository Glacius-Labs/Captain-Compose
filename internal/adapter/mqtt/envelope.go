package mqtt

import "encoding/json"

const (
	TypeCreate = "create"
	TypeRemove = "remove"
)

type Envelope struct {
	ID   string          `json:"id"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}
