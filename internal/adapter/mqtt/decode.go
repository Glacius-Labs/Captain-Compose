package mqtt

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"

	"github.com/glacius-labs/captain-compose/internal/app/deployment/create"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/remove"
	"github.com/glacius-labs/captain-compose/internal/control"
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
	var envelopeFields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &envelopeFields); err != nil {
		return e, fmt.Errorf("invalid envelope")
	}
	if version, present := envelopeFields["version"]; present && bytes.Equal(bytes.TrimSpace(version), []byte("null")) {
		return e, fmt.Errorf("version cannot be null")
	}
	if expires, present := envelopeFields["expires_at"]; present && bytes.Equal(bytes.TrimSpace(expires), []byte("null")) {
		return e, fmt.Errorf("expires_at cannot be null")
	}
	id, err := uuid.Parse(e.ID)
	if err != nil || id == uuid.Nil {
		return e, fmt.Errorf("id must be a UUID")
	}
	e.ID = id.String()
	if e.Version != 0 && e.Version != 1 && e.Version != 2 {
		return e, fmt.Errorf("unsupported protocol version")
	}
	if e.ExpiresAt != nil {
		_, offset := e.ExpiresAt.Zone()
		if e.Version != 2 || e.ExpiresAt.IsZero() || offset != 0 {
			return e, fmt.Errorf("expires_at must be a UTC timestamp in version 2")
		}
	}
	if e.Version == 2 {
		return decodeV2(e)
	}
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

func decodeV2(e Envelope) (Envelope, error) {
	trimmed := bytes.TrimSpace(e.Data)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return e, fmt.Errorf("data must be a JSON object")
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &fields); err != nil {
		return e, fmt.Errorf("data must be a JSON object")
	}
	var req control.Request
	if err := strictJSON(e.Data, &req); err != nil {
		return e, err
	}
	allowed := map[string]bool{}
	requiresName := false
	requiresPayload := false
	switch e.Type {
	case "create", "plan":
		allowed = map[string]bool{"name": true, "payload": true, "expected_revision": true}
		requiresName, requiresPayload = true, true
	case "remove":
		allowed = map[string]bool{"name": true, "expected_revision": true}
		requiresName = true
	case "status":
		allowed = map[string]bool{"name": true}
	case "inspect":
		allowed = map[string]bool{"name": true}
		requiresName = true
	case "doctor":
	case "result":
		allowed = map[string]bool{"request_id": true}
		id, err := uuid.Parse(req.ResultID)
		if err != nil || id == uuid.Nil {
			return e, fmt.Errorf("request_id must be a UUID")
		}
		req.ResultID = id.String()
	case "revert":
		allowed = map[string]bool{"name": true, "revision": true, "expected_revision": true, "allow_data_risk": true}
		requiresName = true
	default:
		return e, fmt.Errorf("unsupported command type")
	}
	for field, raw := range fields {
		if !allowed[field] {
			return e, fmt.Errorf("%s is not valid for this action", field)
		}
		if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return e, fmt.Errorf("%s cannot be null", field)
		}
	}
	if isQueryAction(e.Type) && e.ExpiresAt != nil {
		return e, fmt.Errorf("query requests do not use expires_at")
	}
	if req.Name != "" {
		if _, allowedName := allowed["name"]; !allowedName {
			return e, fmt.Errorf("name is not valid for this action")
		}
		if err := deployment.ValidateName(req.Name); err != nil {
			return e, err
		}
	}
	if requiresName && req.Name == "" {
		return e, fmt.Errorf("name is required")
	}
	if requiresPayload {
		var encoded string
		if err := json.Unmarshal(fields["payload"], &encoded); err != nil {
			return e, fmt.Errorf("payload must be base64 text")
		}
		payload, err := base64.StdEncoding.Strict().DecodeString(encoded)
		if err != nil || base64.StdEncoding.EncodeToString(payload) != encoded {
			return e, fmt.Errorf("payload must be canonical standard base64")
		}
		req.Payload = payload
		if err := deployment.ValidatePayload(req.Payload); err != nil {
			return e, err
		}
	} else if len(req.Payload) != 0 {
		return e, fmt.Errorf("payload is not valid for this action")
	}
	if req.ExpectedRevision != nil && *req.ExpectedRevision != "" && !validRevision(*req.ExpectedRevision) {
		return e, fmt.Errorf("expected_revision must be an empty string or lowercase SHA-256")
	}
	if e.Type == "remove" && req.ExpectedRevision != nil && *req.ExpectedRevision == "" {
		return e, fmt.Errorf("empty expected_revision is only valid for create or plan")
	}
	if req.Revision != "" && !validRevision(req.Revision) {
		return e, fmt.Errorf("revision must be lowercase SHA-256")
	}
	if e.Type == "revert" && (!req.AllowDataRisk || !validRevision(req.Revision) || req.ExpectedRevision == nil || !validRevision(*req.ExpectedRevision)) {
		return e, fmt.Errorf("revert requires expected_revision, revision and allow_data_risk acknowledgement")
	}
	req.RequestID = e.ID
	e.Data, _ = json.Marshal(req)
	return e, nil
}

func validRevision(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		if !(c >= '0' && c <= '9') && !(c >= 'a' && c <= 'f') {
			return false
		}
	}
	return true
}
