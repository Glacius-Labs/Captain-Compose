package operator

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/glacius-labs/captain-compose/internal/control"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type fakeExchange struct {
	calls              []Envelope
	saved              map[string]Event
	doctorChecks       []control.Check
	pendingNext        int
	malformedResult    bool
	wrongResultVersion bool
}

func (f *fakeExchange) exchange(ctx context.Context, env Environment, e Envelope, retry time.Duration, wait bool) (Outcome, error) {
	f.calls = append(f.calls, e)
	var event Event
	if e.Type == "result" {
		original := f.saved[e.Data.ResultID]
		if f.malformedResult {
			original.RequestID = "another-request"
		}
		if f.wrongResultVersion {
			original.Version = 3
		}
		if f.pendingNext > 0 {
			f.pendingNext--
			data, _ := json.Marshal(map[string]any{"status": "pending", "result": nil})
			event = Event{Version: 2, ID: uuid.NewString(), RequestID: e.ID, Action: "result", Success: true, Code: "pending", Message: "still working", Data: data}
		} else {
			data, _ := json.Marshal(map[string]any{"status": "delivered", "result": original})
			event = Event{Version: 2, ID: uuid.NewString(), RequestID: e.ID, Action: "result", Success: true, Code: "delivered", Data: data}
		}
	} else {
		var data any
		switch e.Type {
		case "plan":
			data = control.Plan{Name: e.Data.Name, Revision: "rev", Added: []string{"web"}}
		case "status":
			data = []control.DeploymentStatus{{Name: e.Data.Name, Phase: "running"}}
		case "inspect":
			data = control.DeploymentStatus{Name: e.Data.Name, Phase: "running"}
		case "doctor":
			checks := f.doctorChecks
			if checks == nil {
				checks = []control.Check{{Name: "mqtt", OK: true, Message: "round trip"}}
			}
			data = checks
		}
		raw, _ := json.Marshal(data)
		event = Event{Version: 2, ID: uuid.NewString(), RequestID: e.ID, Action: e.Type, Name: e.Data.Name, Success: true, Code: "ok", Data: raw}
		f.saved[e.ID] = event
	}
	transport := newFakeMQTT(1)
	transport.onPublish = func([]byte) Event { return event }
	return exchangeWithTransport(ctx, env, e, retry, wait, transport)
}

func TestDocumentedCommandsAndResultRoundTrip(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(compose, []byte("services:\n  web:\n    image: nginx\n"), 0600))
	env := Environment{BrokerURL: "ssl://mqtt.example.test:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events", Timeout: 10 * time.Second}
	fake := &fakeExchange{saved: map[string]Event{}}
	run := func(command string, args ...string) (int, string, string) {
		t.Helper()
		out, errOut := new(strings.Builder), new(strings.Builder)
		code := runCommand(command, args, "pilot", env, true, out, errOut, fake.exchange)
		return code, out.String(), errOut.String()
	}

	code, out, _ := run("deploy", "web", compose)
	require.Equal(t, ExitOK, code)
	require.Contains(t, out, "\"state\":\"completed\"")
	deployCall := fake.calls[len(fake.calls)-1]
	require.Equal(t, "create", deployCall.Type)
	require.Equal(t, 2, deployCall.Version)
	require.NotEmpty(t, deployCall.ExpiresAt)
	originalID := deployCall.ID

	for _, tc := range []struct {
		command string
		args    []string
		kind    string
	}{
		{"plan", []string{"web", compose}, "plan"},
		{"status", nil, "status"},
		{"status", []string{"web"}, "status"},
		{"inspect", []string{"web"}, "inspect"},
		{"remove", []string{"web"}, "remove"},
		{"doctor", nil, "doctor"},
		{"revert", []string{"web", "--revision", hash('a'), "--expected-revision", hash('b'), "--allow-data-risk"}, "revert"},
	} {
		code, _, stderr := run(tc.command, tc.args...)
		require.Equal(t, ExitOK, code, tc.command+" stderr="+stderr)
		call := fake.calls[len(fake.calls)-1]
		require.Equal(t, tc.kind, call.Type)
		if tc.kind == "plan" || tc.kind == "status" || tc.kind == "inspect" || tc.kind == "doctor" {
			require.Empty(t, call.ExpiresAt, tc.kind+" must omit expires_at")
		}
	}
	code, out, _ = run("result", originalID)
	require.Equal(t, ExitOK, code)
	require.Contains(t, out, originalID)
	require.Contains(t, out, "delivered")
	query1 := fake.calls[len(fake.calls)-1]
	code, _, _ = run("wait", originalID)
	require.Equal(t, ExitOK, code)
	query2 := fake.calls[len(fake.calls)-1]
	require.NotEqual(t, query1.ID, query2.ID)
	require.Equal(t, originalID, query2.Data.ResultID)
}

func TestResultIsOneShotAndWaitPollsWithFreshQueryIDs(t *testing.T) {
	fake := &fakeExchange{saved: map[string]Event{}}
	targetID := uuid.NewString()
	original := Event{Version: 2, ID: uuid.NewString(), RequestID: targetID, Action: "create", Success: true, Code: "ok"}
	fake.saved[original.RequestID] = original
	env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events", Timeout: time.Second}
	fake.pendingNext = 1
	out := new(strings.Builder)
	code := runCommand("result", []string{targetID}, "pilot", env, true, out, io.Discard, fake.exchange)
	require.Equal(t, ExitUnknown, code)
	require.Contains(t, out.String(), `"state":"pending"`)
	require.Contains(t, out.String(), original.RequestID)
	oneShotQuery := fake.calls[0].ID
	fake.pendingNext = 1
	out.Reset()
	code = runCommand("wait", []string{targetID, "--retry-interval=1ms"}, "pilot", env, true, out, io.Discard, fake.exchange)
	require.Equal(t, ExitOK, code)
	require.Contains(t, out.String(), `"state":"delivered"`)
	require.Len(t, fake.calls, 3)
	require.NotEqual(t, oneShotQuery, fake.calls[1].ID)
	require.NotEqual(t, fake.calls[1].ID, fake.calls[2].ID)
}

func TestSavedResultMustMatchOriginalRequestAndV2(t *testing.T) {
	for _, mutate := range []func(*fakeExchange){func(f *fakeExchange) { f.malformedResult = true }, func(f *fakeExchange) { f.wrongResultVersion = true }} {
		fake := &fakeExchange{saved: map[string]Event{}}
		mutate(fake)
		originalID := uuid.NewString()
		fake.saved[originalID] = Event{Version: 2, ID: uuid.NewString(), RequestID: originalID, Action: "create", Success: true, Code: "ok"}
		env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events"}
		out := new(strings.Builder)
		code := runCommand("result", []string{originalID}, "pilot", env, true, out, io.Discard, fake.exchange)
		require.Equal(t, ExitRemote, code)
		require.Contains(t, out.String(), "invalid_result")
		require.Contains(t, out.String(), `"ok":false`)
	}
}

func TestWaitTimeoutKeepsOriginalRequestID(t *testing.T) {
	fake := &fakeExchange{saved: map[string]Event{}, pendingNext: 100}
	originalID := uuid.NewString()
	env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events", Timeout: time.Millisecond}
	out, errOut := new(strings.Builder), new(strings.Builder)
	code := runCommand("wait", []string{originalID, "--retry-interval=1ms"}, "pilot", env, true, out, errOut, fake.exchange)
	require.Equal(t, ExitUnknown, code)
	require.Contains(t, errOut.String(), originalID)
}

func TestEmptyExpectedRevisionIsRejectedLocallyForRemove(t *testing.T) {
	fake := &fakeExchange{saved: map[string]Event{}}
	env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events"}
	out, errOut := new(strings.Builder), new(strings.Builder)
	code := runCommand("remove", []string{"web", "--expected-revision", ""}, "pilot", env, true, out, errOut, fake.exchange)
	require.Equal(t, ExitUsage, code)
	require.Empty(t, fake.calls)
	require.Contains(t, errOut.String(), "empty --expected-revision is only valid for deploy or plan")
}

func TestCreateOnlyUsesEmptyPreconditionWithoutEmptyRevisionFlag(t *testing.T) {
	fake := &fakeExchange{saved: map[string]Event{}}
	env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events"}
	dir := t.TempDir()
	compose := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(compose, []byte("services: {}"), 0600))
	out := new(strings.Builder)
	code := runCommand("deploy", []string{"web", compose, "--create-only"}, "pilot", env, true, out, io.Discard, fake.exchange)
	require.Equal(t, ExitOK, code)
	require.NotNil(t, fake.calls[0].Data.ExpectedRevision)
	require.Empty(t, *fake.calls[0].Data.ExpectedRevision)
	fake.calls = nil
	out.Reset()
	code = runCommand("deploy", []string{"web", compose, "--expected-revision", ""}, "pilot", env, true, out, io.Discard, fake.exchange)
	require.Equal(t, ExitOK, code)
	require.NotNil(t, fake.calls[0].Data.ExpectedRevision)
	require.Empty(t, *fake.calls[0].Data.ExpectedRevision)
}

func TestSavedV1ResultsRemainReadable(t *testing.T) {
	for _, version := range []int{0, 1} {
		fake := &fakeExchange{saved: map[string]Event{}}
		originalID := uuid.NewString()
		action := "create"
		if version == 1 {
			action = "delete"
		}
		fake.saved[originalID] = Event{Version: version, ID: uuid.NewString(), RequestID: originalID, Action: action, Success: true, Message: "legacy result"}
		env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events"}
		out := new(strings.Builder)
		code := runCommand("result", []string{originalID}, "pilot", env, true, out, io.Discard, fake.exchange)
		require.Equal(t, ExitOK, code)
		require.Contains(t, out.String(), "legacy result")
	}
}

func TestHelpReturnsSuccessAndMissingArgumentsReturnUsage(t *testing.T) {
	out, errOut := new(strings.Builder), new(strings.Builder)
	require.Equal(t, ExitOK, Run([]string{"--help"}, out, errOut))
	env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events"}
	require.Equal(t, ExitOK, runCommand("deploy", []string{"--help"}, "pilot", env, true, out, errOut, (&fakeExchange{saved: map[string]Event{}}).exchange))
	require.Equal(t, ExitUsage, runCommand("deploy", nil, "pilot", env, true, out, errOut, (&fakeExchange{saved: map[string]Event{}}).exchange))
}

func TestRequestFileCreateReplayAndRequestIDRetry(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "compose.yaml")
	recordPath := filepath.Join(dir, "request.json")
	require.NoError(t, os.WriteFile(compose, []byte("services: {}\n"), 0600))
	env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events"}
	fake := &fakeExchange{saved: map[string]Event{}}
	run := func(args ...string) (int, string) {
		out := new(strings.Builder)
		code := runCommand("deploy", args, "pilot", env, true, out, io.Discard, fake.exchange)
		return code, out.String()
	}
	code, _ := run("web", compose, "--request-file", recordPath)
	require.Equal(t, ExitOK, code)
	first := fake.calls[len(fake.calls)-1]
	info, err := os.Lstat(recordPath)
	require.NoError(t, err)
	require.True(t, info.Mode().IsRegular())
	code, _ = run("web", compose, "--request-file", recordPath)
	require.Equal(t, ExitOK, code)
	second := fake.calls[len(fake.calls)-1]
	require.Equal(t, first.ID, second.ID)
	require.Equal(t, first.ExpiresAt, second.ExpiresAt)
	_, err = os.Stat(recordPath)
	require.NoError(t, err)
	require.Error(t, writeRequestRecord(recordPath, requestRecord{}), "exclusive create must not overwrite existing request record")
	id := "123e4567-e89b-12d3-a456-426614174000"
	code, _ = run("web", compose, "--request-id", id)
	require.Equal(t, ExitOK, code)
	require.Empty(t, fake.calls[len(fake.calls)-1].ExpiresAt)
	code, _ = run("web", compose, "--request-id", id)
	require.Equal(t, ExitOK, code)
	require.Equal(t, id, fake.calls[len(fake.calls)-1].ID)
}

func TestRequestFileStrictAndEnvironmentBound(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "compose.yaml")
	path := filepath.Join(dir, "request.json")
	require.NoError(t, os.WriteFile(compose, []byte("services: {}"), 0600))
	require.NoError(t, os.WriteFile(path, []byte(`{"environment":"pilot","broker_url":"ssl://broker:8883","command_topic":"c","event_topic":"e","envelope":{},"extra":true}`), 0600))
	env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events"}
	fake := &fakeExchange{saved: map[string]Event{}}
	out := new(strings.Builder)
	code := runCommand("deploy", []string{"web", compose, "--request-file", path}, "pilot", env, true, out, io.Discard, fake.exchange)
	require.Equal(t, ExitLocal, code)
	require.Empty(t, fake.calls)
	valid, _ := json.Marshal(requestRecord{Environment: "pilot", BrokerURL: "ssl://different:8883", CommandTopic: "c", EventTopic: "e", Envelope: NewEnvelope("create", control.Request{Name: "web", Payload: []byte("services: {}")}, time.Minute)})
	require.NoError(t, os.WriteFile(path, valid, 0600))
	out.Reset()
	code = runCommand("deploy", []string{"web", compose, "--request-file", path}, "pilot", env, true, out, io.Discard, fake.exchange)
	require.Equal(t, ExitUsage, code)
	require.Empty(t, fake.calls)
}

func TestDoctorFailureIsPreservedThroughSavedResult(t *testing.T) {
	fake := &fakeExchange{saved: map[string]Event{}, doctorChecks: []control.Check{{Name: "broker", OK: false, Message: "unavailable"}}}
	env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events"}
	out := new(strings.Builder)
	code := runCommand("doctor", nil, "pilot", env, true, out, io.Discard, fake.exchange)
	require.Equal(t, ExitRemote, code)
	require.Contains(t, out.String(), `"ok":false`)
	id := fake.calls[0].ID
	out.Reset()
	code = runCommand("result", []string{id}, "pilot", env, true, out, io.Discard, fake.exchange)
	require.Equal(t, ExitRemote, code)
	require.Contains(t, out.String(), `"ok":false`)
	require.Contains(t, out.String(), "doctor_failed")
}

func hash(ch byte) string { return strings.Repeat(string(ch), 64) }
