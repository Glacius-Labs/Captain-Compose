package mqtt

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

	"github.com/glacius-labs/captain-compose/internal/app/deployment/create"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/remove"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/google/uuid"
)

const MaxMessageBytes = 1500 * 1024

func strictJSON(raw []byte, value any) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return fmt.Errorf("invalid JSON or unsupported fields")
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return fmt.Errorf("expected one JSON object")
	}
	return nil
}

func Decode(raw []byte) (Envelope, error) {
	var e Envelope
	if len(raw) > MaxMessageBytes {
		return e, fmt.Errorf("message exceeds %d bytes", MaxMessageBytes)
	}
	if err := strictJSON(raw, &e); err != nil {
		return e, err
	}
	id, err := uuid.Parse(e.ID)
	if err != nil || id == uuid.Nil {
		return e, fmt.Errorf("id must be a UUID")
	}
	e.ID = id.String()
	switch e.Type {
	case TypeCreate:
		var c create.Command
		if err := strictJSON(e.Data, &c); err != nil {
			return e, err
		}
		if err := deployment.ValidateName(c.Name); err != nil {
			return e, err
		}
		if err := deployment.ValidatePayload(c.Payload); err != nil {
			return e, err
		}
		e.Data, _ = json.Marshal(c)
	case TypeRemove:
		var c remove.Command
		if err := strictJSON(e.Data, &c); err != nil {
			return e, err
		}
		if err := deployment.ValidateName(c.Name); err != nil {
			return e, err
		}
		e.Data, _ = json.Marshal(c)
	default:
		return e, fmt.Errorf("unsupported command type")
	}
	return e, nil
}
