package operator

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/glacius-labs/captain-compose/internal/control"
	"github.com/stretchr/testify/require"
)

func TestEnvelopeV2EncodingAndUniqueIDs(t *testing.T) {
	request := control.Request{Name: "web", Payload: []byte("services: {}"), AllowDataRisk: true}
	a := NewEnvelope("create", request, time.Minute)
	b := NewEnvelope("create", request, time.Minute)
	require.NotEqual(t, a.ID, b.ID)
	require.Equal(t, 2, a.Version)
	require.Equal(t, "create", a.Type)
	require.NotEmpty(t, a.ExpiresAt)
	var decoded map[string]any
	require.NoError(t, json.Unmarshal(mustJSON(t, a), &decoded))
	require.Equal(t, float64(2), decoded["version"])
	require.Equal(t, a.ID, decoded["id"])
	require.NotContains(t, decoded, "reply_to")
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

func TestTLSRequiresClientCertificatePairAndKeepsVerification(t *testing.T) {
	_, err := newTLSConfig(TLSConfig{ClientCertPath: "only-cert.pem"})
	require.ErrorContains(t, err, "must be configured together")
	tlsCfg, err := newTLSConfig(TLSConfig{})
	require.NoError(t, err)
	require.Equal(t, uint16(0x0303), tlsCfg.MinVersion)
	require.False(t, tlsCfg.InsecureSkipVerify)
}

func TestDoctorCheckEvaluationFailsClosed(t *testing.T) {
	_, failed := doctorChecks(json.RawMessage(`[{"name":"broker","ok":true,"message":"connected"}]`))
	require.False(t, failed)
	_, failed = doctorChecks(json.RawMessage(`[{"name":"docker","ok":false,"message":"unavailable"}]`))
	require.True(t, failed)
	_, failed = doctorChecks(json.RawMessage(`{}`))
	require.True(t, failed)
}

func TestValidRevisionRequiresLowercaseSha256(t *testing.T) {
	require.True(t, validRevision("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"))
	require.False(t, validRevision("0123456789ABCDEF0123456789abcdef0123456789abcdef0123456789abcdef"))
	require.False(t, validRevision("short"))
}
