package mqtt

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/glacius-labs/captain-compose/internal/adapter/mock"
	"github.com/glacius-labs/captain-compose/internal/control"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

type v2RuntimeStub struct {
	mock.Runtime
	mu             sync.Mutex
	calls          []control.Request
	gate           chan struct{}
	value          any
	callActions    chan string
	blockMutations bool
}

func (r *v2RuntimeStub) Execute(ctx context.Context, action string, request control.Request) (any, error) {
	if request.RequestID == "" {
		return nil, errors.New("request id was not propagated")
	}
	r.mu.Lock()
	r.calls = append(r.calls, request)
	r.mu.Unlock()
	if r.callActions != nil {
		select {
		case r.callActions <- action:
		default:
		}
	}
	if r.blockMutations && (action == "create" || action == "remove" || action == "revert") {
		<-ctx.Done()
		return nil, ctx.Err()
	}
	if r.gate != nil {
		select {
		case r.gate <- struct{}{}:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
	if r.value != nil {
		return r.value, nil
	}
	return map[string]any{"action": action, "request_id": request.RequestID}, nil
}

func v2Command(t *testing.T, action, name string) Envelope {
	t.Helper()
	data, err := json.Marshal(control.Request{Name: name})
	require.NoError(t, err)
	e, err := Decode(mustJSON(t, map[string]any{"id": uuid.NewString(), "version": 2, "type": action, "data": json.RawMessage(data)}))
	require.NoError(t, err)
	return e
}

func mustJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	require.NoError(t, err)
	return b
}

type v2Publisher struct {
	events          chan deployment.Event
	err             error
	once            sync.Once
	cancel          context.CancelFunc
	cancelRequestID string
}

func (p *v2Publisher) Publish(_ context.Context, e deployment.Event) error {
	select {
	case p.events <- e:
	default:
	}
	if p.cancel != nil && (p.cancelRequestID == "" || p.cancelRequestID == e.RequestID) {
		p.once.Do(p.cancel)
	}
	return p.err
}

func TestV2ExecutionContinuesWhileOutboxPublicationFails(t *testing.T) {
	rt := &v2RuntimeStub{gate: make(chan struct{}, 4)}
	pub := &v2Publisher{events: make(chan deployment.Event, 8), err: errors.New("broker offline")}
	l, err := NewListener("commands", t.TempDir(), rt, pub)
	require.NoError(t, err)
	defer l.Close()
	first, second := v2Command(t, "remove", "first"), v2Command(t, "remove", "second")
	require.NoError(t, l.journal.enqueue(first))
	require.NoError(t, l.journal.enqueue(second))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- l.Start(ctx) }()
	for range 2 {
		select {
		case <-rt.gate:
		case <-time.After(2 * time.Second):
			cancel()
			t.Fatal("execution stalled behind outbox delivery")
		}
	}
	cancel()
	require.NoError(t, <-done)
	rt.mu.Lock()
	defer rt.mu.Unlock()
	require.Len(t, rt.calls, 2)
	require.Equal(t, first.ID, rt.calls[0].RequestID)
	require.Equal(t, second.ID, rt.calls[1].RequestID)
	stats := l.Stats()
	require.Equal(t, 2, stats.Outbox)
}

func TestV2QueryBypassesBlockedMutationExecutor(t *testing.T) {
	rt := &v2RuntimeStub{callActions: make(chan string, 4), blockMutations: true}
	pub := &v2Publisher{events: make(chan deployment.Event, 4)}
	l, err := NewListener("commands", t.TempDir(), rt, pub)
	require.NoError(t, err)
	defer l.Close()
	mutation := v2Command(t, "remove", "web")
	require.NoError(t, l.journal.enqueue(mutation))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- l.Start(ctx) }()
	select {
	case action := <-rt.callActions:
		require.Equal(t, "remove", action)
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("mutation executor did not start")
	}
	query := v2Command(t, "status", "")
	pub.cancel = cancel
	pub.cancelRequestID = query.ID
	b, err := json.Marshal(query)
	require.NoError(t, err)
	msg := &message{payload: b, qos: 1}
	l.HandleMessage(nil, msg)
	require.True(t, msg.ack)
	select {
	case action := <-rt.callActions:
		require.Equal(t, "status", action)
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("status query waited behind blocked mutation")
	}
	select {
	case event := <-pub.events:
		require.Equal(t, query.ID, event.RequestID)
		require.Equal(t, "status", event.Action)
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("status query result was not delivered")
	}
	require.NoError(t, <-done)
}

func TestV2ExpiredMutationIsJournaledWithoutRuntimeCall(t *testing.T) {
	rt := &v2RuntimeStub{}
	pub := &v2Publisher{events: make(chan deployment.Event, 1)}
	l, err := NewListener("commands", t.TempDir(), rt, pub)
	require.NoError(t, err)
	defer l.Close()
	expired := time.Now().UTC().Add(-time.Minute)
	e, err := Decode(mustJSON(t, map[string]any{"id": uuid.NewString(), "version": 2, "type": "remove", "expires_at": expired, "data": map[string]any{"name": "web"}}))
	require.NoError(t, err)
	require.NoError(t, l.journal.enqueue(e))
	ctx, cancel := context.WithCancel(context.Background())
	pub.cancel = cancel
	require.NoError(t, l.Start(ctx))
	got := <-pub.events
	require.Equal(t, 2, got.Version)
	require.Equal(t, "expired", got.Code)
	require.False(t, got.Success)
	rt.mu.Lock()
	require.Empty(t, rt.calls)
	rt.mu.Unlock()
}

func TestStartedMutationResumesAfterExpiryWithoutPrematureFailure(t *testing.T) {
	dir := t.TempDir()
	expires := time.Now().UTC().Add(150 * time.Millisecond)
	e, err := Decode(mustJSON(t, map[string]any{"id": uuid.NewString(), "version": 2, "type": "remove", "expires_at": expires, "data": map[string]any{"name": "web"}}))
	require.NoError(t, err)
	rt := &v2RuntimeStub{callActions: make(chan string, 2), blockMutations: true}
	l, err := NewListener("commands", dir, rt, &v2Publisher{events: make(chan deployment.Event, 2)})
	require.NoError(t, err)
	require.NoError(t, l.journal.enqueue(e))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- l.Start(ctx) }()
	select {
	case action := <-rt.callActions:
		require.Equal(t, "remove", action)
	case <-time.After(2 * time.Second):
		cancel()
		t.Fatal("first execution did not begin")
	}
	cancel()
	require.NoError(t, <-done)
	record, found := l.journal.find(e.ID)
	require.True(t, found)
	require.NotNil(t, record.Started)
	require.Nil(t, record.Event)
	require.NoError(t, l.Close())
	if wait := time.Until(expires.Add(25 * time.Millisecond)); wait > 0 {
		time.Sleep(wait)
	}

	rt = &v2RuntimeStub{}
	pub := &v2Publisher{events: make(chan deployment.Event, 2)}
	l, err = NewListener("commands", dir, rt, pub)
	require.NoError(t, err)
	defer l.Close()
	ctx, cancel = context.WithCancel(context.Background())
	pub.cancel = cancel
	pub.cancelRequestID = e.ID
	require.NoError(t, l.Start(ctx))
	got := <-pub.events
	require.Equal(t, "ok", got.Code)
	require.True(t, got.Success)
	require.Len(t, rt.calls, 1)
}

func TestV2ResultLookupReturnsLifecycleAndSavedEvent(t *testing.T) {
	l, err := NewListener("commands", t.TempDir(), &v2RuntimeStub{}, &v2Publisher{events: make(chan deployment.Event, 2)})
	require.NoError(t, err)
	defer l.Close()
	target := v2Command(t, "remove", "web")
	require.NoError(t, l.journal.enqueue(target))
	_, ok := l.journal.find(target.ID)
	require.True(t, ok)
	saved := deployment.Event{Version: 2, ID: uuid.New(), RequestID: target.ID, Timestamp: time.Now().UTC(), Action: "remove", Name: "web", Success: true, Code: "ok", Message: "payload secret should not escape", Labels: map[string]string{"error": "raw detail"}, Data: map[string]any{"payload": "compose"}}
	now := time.Now().UTC()
	require.NoError(t, l.journal.saveEvent(target.ID, saved, &now, true))
	require.NoError(t, l.journal.markDelivered(target.ID, now))
	data, err := json.Marshal(control.Request{ResultID: target.ID})
	require.NoError(t, err)
	query, err := Decode(mustJSON(t, map[string]any{"id": uuid.NewString(), "version": 2, "type": "result", "data": json.RawMessage(data)}))
	require.NoError(t, err)
	require.NoError(t, l.journal.enqueue(query))
	ctx, cancel := context.WithCancel(context.Background())
	pub := &v2Publisher{events: make(chan deployment.Event, 2), cancel: cancel}
	l.publisher = pub
	require.NoError(t, l.Start(ctx))
	got := <-pub.events
	require.Equal(t, "delivered", got.Code)
	dataMap, ok := got.Data.(map[string]any)
	require.True(t, ok)
	require.Equal(t, "delivered", dataMap["status"])
	result, ok := dataMap["result"].(map[string]any)
	require.True(t, ok)
	require.Equal(t, target.ID, result["request_id"])
	require.Nil(t, result["labels"])
	require.Nil(t, result["data"])
	require.Equal(t, "Saved operation succeeded", result["message"])
}

func TestV2OversizedQueryUsesFixedResponse(t *testing.T) {
	rt := &v2RuntimeStub{value: map[string]any{"items": strings.Repeat("x", MaxEventBytes)}}
	pub := &v2Publisher{events: make(chan deployment.Event, 1)}
	l, err := NewListener("commands", t.TempDir(), rt, pub)
	require.NoError(t, err)
	defer l.Close()
	e := v2Command(t, "doctor", "")
	require.NoError(t, l.journal.enqueue(e))
	ctx, cancel := context.WithCancel(context.Background())
	pub.cancel = cancel
	require.NoError(t, l.Start(ctx))
	got := <-pub.events
	require.Equal(t, "response_too_large", got.Code)
	require.False(t, got.Success)
	require.Nil(t, got.Data)
	require.Contains(t, got.Message, "narrow the query")
}

func TestV2ConflictingIDAndLegacyCompletedJournalRecovery(t *testing.T) {
	dir := t.TempDir()
	j, err := openJournal(dir)
	require.NoError(t, err)
	e := v2Command(t, "remove", "one")
	require.NoError(t, j.enqueue(e))
	changed := e
	changed.Data = json.RawMessage(`{"name":"two","request_id":"` + uuid.NewString() + `"}`)
	require.ErrorIs(t, j.enqueue(changed), ErrIDConflict)
	require.NoError(t, j.root.Close())
	legacyDir := t.TempDir()
	legacy, err := openJournal(legacyDir)
	require.NoError(t, err)
	defer legacy.root.Close()
	old := command()
	require.NoError(t, legacy.enqueue(old))
	r, ok := legacy.next()
	require.True(t, ok)
	ev := deployment.NewRemovedEvent("web")
	ev.RequestID = old.ID
	now := time.Now().UTC()
	r.Event, r.Completed, r.Command = &ev, &now, nil
	require.NoError(t, legacy.update(r))
	require.NoError(t, legacy.root.Close())
	legacy, err = openJournal(legacyDir)
	require.NoError(t, err)
	defer legacy.root.Close()
	_, pending := legacy.nextOutbox()
	require.False(t, pending)
	_, pending = legacy.nextExecution()
	require.False(t, pending)
}

func TestLegacyOnDiskCommandRecordStillLoads(t *testing.T) {
	dir := t.TempDir()
	e := command()
	commandBytes, err := json.Marshal(e)
	require.NoError(t, err)
	digest := sha256.Sum256(commandBytes)
	received := time.Now().UTC()
	recBytes, err := json.Marshal(map[string]any{
		"id": e.ID, "hash": fmt.Sprintf("%x", digest), "sequence": uint64(1),
		"received": received, "command": e,
	})
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, journalName(e.ID)), recBytes, 0600))
	j, err := openJournal(dir)
	require.NoError(t, err)
	defer j.root.Close()
	loaded, ok := j.nextExecution()
	require.True(t, ok)
	require.Equal(t, e.ID, loaded.ID)
	require.Zero(t, loaded.Command.Version)
}

func TestV2RetryCannotOverrunBoundedOutbox(t *testing.T) {
	j, err := openJournal(t.TempDir())
	require.NoError(t, err)
	defer j.root.Close()
	retry := v2Command(t, "remove", "original")
	require.NoError(t, j.enqueue(retry))
	event := deployment.Event{Version: 2, ID: uuid.New(), RequestID: retry.ID, Timestamp: time.Now().UTC(), Action: "remove", Name: "original", Success: true, Code: "ok", Message: "Operation completed"}
	now := time.Now().UTC()
	require.NoError(t, j.saveEvent(retry.ID, event, &now, true))
	require.NoError(t, j.markDelivered(retry.ID, now))
	for i := 0; i < maxPending-reservedQueryCapacity; i++ {
		require.NoError(t, j.enqueue(v2Command(t, "remove", "queued")))
	}
	require.ErrorIs(t, j.enqueue(retry), ErrCapacity)
	got, ok := j.find(retry.ID)
	require.True(t, ok)
	require.NotNil(t, got.Delivered)
}

func TestV2ActionSpecificDecodeRules(t *testing.T) {
	valid := []map[string]any{
		{"type": "create", "data": map[string]any{"name": "web", "payload": "c2VydmljZXM6IHt9"}},
		{"type": "remove", "data": map[string]any{"name": "web", "expected_revision": strings.Repeat("b", 64)}},
		{"type": "plan", "data": map[string]any{"name": "web", "payload": "c2VydmljZXM6IHt9"}},
		{"type": "status", "data": map[string]any{}},
		{"type": "status", "data": map[string]any{"name": "web"}},
		{"type": "status", "data": map[string]any{"name": ""}},
		{"type": "inspect", "data": map[string]any{"name": "web"}},
		{"type": "doctor", "data": map[string]any{}},
		{"type": "revert", "data": map[string]any{"name": "web", "revision": strings.Repeat("a", 64), "expected_revision": strings.Repeat("b", 64), "allow_data_risk": true}},
		{"type": "result", "data": map[string]any{"request_id": uuid.NewString()}},
	}
	for _, item := range valid {
		item["id"], item["version"] = uuid.NewString(), 2
		_, err := Decode(mustJSON(t, item))
		require.NoErrorf(t, err, "action %s", item["type"])
	}
	invalid := []map[string]any{
		{"type": "plan", "data": map[string]any{"name": "web"}},
		{"type": "doctor", "data": map[string]any{"name": "web"}},
		{"type": "inspect", "data": map[string]any{}},
		{"type": "status", "data": map[string]any{"payload": "c2VydmljZXM6IHt9"}},
		{"type": "result", "data": map[string]any{"request_id": uuid.NewString(), "name": "web"}},
		{"type": "remove", "data": map[string]any{"name": "web", "expected_revision": ""}},
	}
	for _, item := range invalid {
		item["id"], item["version"] = uuid.NewString(), 2
		_, err := Decode(mustJSON(t, item))
		require.Errorf(t, err, "action %s should be rejected", item["type"])
	}
	for _, raw := range []string{
		`{"id":"` + uuid.NewString() + `","version":null,"type":"remove","data":{"name":"web"}}`,
		`{"id":"` + uuid.NewString() + `","version":2,"type":"remove","expires_at":null,"data":{"name":"web"}}`,
		`{"id":"` + uuid.NewString() + `","version":2,"type":"remove","data":null}`,
		`{"id":"00000000-0000-0000-0000-000000000000","version":2,"type":"remove","data":{"name":"web"}}`,
		`{"id":"` + uuid.NewString() + `","version":2,"type":"create","data":{"name":"web","payload":"AB=="}}`,
	} {
		_, err := Decode([]byte(raw))
		require.Error(t, err)
	}
}
