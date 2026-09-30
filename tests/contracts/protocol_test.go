package contracts

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	adapter "github.com/glacius-labs/captain-compose/internal/adapter/mqtt"
	"github.com/glacius-labs/captain-compose/internal/control"
	"github.com/glacius-labs/captain-compose/internal/operator"
)

func TestSharedWireFixtures(t *testing.T) {
	data, err := os.ReadFile("wire-fixtures.json")
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Name    string          `json:"name"`
		Valid   bool            `json:"valid"`
		Command json.RawMessage `json:"command"`
	}
	if err := json.Unmarshal(data, &cases); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			_, err := adapter.Decode(tc.Command)
			if (err == nil) != tc.Valid {
				t.Fatalf("expected valid=%v, got error=%v", tc.Valid, err)
			}
		})
	}
}

// Exercise the real operator encoder against the real agent decoder so either
// side changing its contract cannot silently invalidate documented commands.
func TestOperatorRequestsAreAcceptedByAgent(t *testing.T) {
	revision := strings.Repeat("a", 64)
	empty := ""
	payload := []byte("services:\n  web:\n    image: nginx:alpine\n")
	for _, tc := range []struct {
		name, action string
		data         control.Request
		expires      time.Duration
	}{
		{"deploy", "create", control.Request{Name: "web", Payload: payload}, time.Minute},
		{"create-only", "create", control.Request{Name: "web", Payload: payload, ExpectedRevision: &empty}, time.Minute},
		{"update", "create", control.Request{Name: "web", Payload: payload, ExpectedRevision: &revision}, time.Minute},
		{"plan", "plan", control.Request{Name: "web", Payload: payload, ExpectedRevision: &revision}, 0},
		{"status-all", "status", control.Request{}, 0},
		{"status-one", "status", control.Request{Name: "web"}, 0},
		{"inspect", "inspect", control.Request{Name: "web"}, 0},
		{"doctor", "doctor", control.Request{}, 0},
		{"remove", "remove", control.Request{Name: "web", ExpectedRevision: &revision}, time.Minute},
		{"revert", "revert", control.Request{Name: "web", Revision: revision, ExpectedRevision: &revision, AllowDataRisk: true}, time.Minute},
		{"result", "result", control.Request{ResultID: "673fca08-5e71-45f9-bfe3-4cb9b79c4731"}, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := operator.NewEnvelope(tc.action, tc.data, tc.expires)
			encoded, err := json.Marshal(req)
			if err != nil {
				t.Fatal(err)
			}
			decoded, err := adapter.Decode(encoded)
			if err != nil {
				t.Fatalf("operator/agent contract mismatch for %s: %v", tc.name, err)
			}
			if decoded.ID != req.ID || decoded.Type != req.Type || decoded.Version != control.Version {
				t.Fatal("request identity changed")
			}
		})
	}
}
