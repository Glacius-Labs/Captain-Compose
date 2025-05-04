package mqtt

import "encoding/json"

const (
	TypeCreate = "create"
	TypeRemove = "remove"
)

type Envelope struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
}
