package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/glacius-labs/captain-compose/internal/adapter/mock"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func command() Envelope {
	return Envelope{ID: uuid.NewString(), Type: TypeRemove, Data: json.RawMessage(`{"name":"web"}`)}
}

func TestDecodeRejectsInvalidCommands(t *testing.T) {
	for _, input := range []string{`{}`, `null`, `{"id":"x","type":"remove","data":{"name":"web"}}`, `{"id":"` + uuid.NewString() + `","type":"remove","data":{"name":"../escape"}}`, `{"id":"` + uuid.NewString() + `","type":"create","data":{"name":"web","payload":"not-base64"}}`} {
		_, err := Decode([]byte(input))
		require.Error(t, err)
	}
	b, _ := json.Marshal(command())
	_, err := Decode(b)
	require.NoError(t, err)
	_, err = Decode(append(b, []byte(" {}")...))
	require.Error(t, err)
	_, err = Decode(make([]byte, MaxMessageBytes+1))
	require.Error(t, err)
}

func TestJournalRecoveryDeduplicationAndConflict(t *testing.T) {
	dir := t.TempDir()
	j, err := openJournal(dir)
	require.NoError(t, err)
	e := command()
	require.NoError(t, j.enqueue(e))
	require.NoError(t, j.enqueue(e))
	require.Len(t, j.records, 1)
	changed := e
	changed.Data = json.RawMessage(`{"name":"other"}`)
	require.Error(t, j.enqueue(changed))
	require.NoError(t, j.root.Close())
	j, err = openJournal(dir)
	require.NoError(t, err)
	defer j.root.Close()
	r, ok := j.next()
	require.True(t, ok)
	require.Equal(t, e.ID, r.ID)
	event := deployment.NewRemovedEvent("web")
	event.RequestID = e.ID
	r.Event = &event
	require.NoError(t, j.update(r))
	now := time.Now()
	r.Completed = &now
	r.Command = nil
	require.NoError(t, j.update(r))
	require.NoError(t, j.enqueue(e))
	_, ok = j.next()
	require.False(t, ok)
}

func TestJournalPreservesEnqueueOrderAcrossTimestampChangesAndRestart(t *testing.T) {
	for _, tc := range []struct {
		name string
		set  func(time.Time, *record, *record)
	}{
		{
			name: "equal timestamps",
			set: func(now time.Time, first, second *record) {
				first.Received = now
				second.Received = now
			},
		},
		{
			name: "clock moved backwards",
			set: func(now time.Time, first, second *record) {
				first.Received = now
				second.Received = now.Add(-time.Minute)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			j, err := openJournal(dir)
			require.NoError(t, err)
			first := command()
			first.ID = "f0000000-0000-4000-8000-000000000001"
			second := command()
			second.ID = "00000000-0000-4000-8000-000000000002"
			require.NoError(t, j.enqueue(first))
			require.NoError(t, j.enqueue(second))

			firstRecord := j.records[first.ID]
			secondRecord := j.records[second.ID]
			tc.set(time.Now().UTC(), &firstRecord, &secondRecord)
			require.NoError(t, j.update(firstRecord))
			require.NoError(t, j.update(secondRecord))
			require.NoError(t, j.root.Close())

			j, err = openJournal(dir)
			require.NoError(t, err)
			defer j.root.Close()
			next, ok := j.next()
			require.True(t, ok)
			require.Equal(t, first.ID, next.ID)

			event := deployment.NewRemovedEvent("web")
			event.RequestID = next.ID
			next.Event = &event
			now := time.Now().UTC()
			next.Completed = &now
			next.Command = nil
			require.NoError(t, j.update(next))
			following, ok := j.next()
			require.True(t, ok)
			require.Equal(t, second.ID, following.ID)

			third := command()
			require.NoError(t, j.enqueue(third))
			require.Greater(t, j.records[third.ID].Sequence, following.Sequence)
		})
	}
}

func TestJournalFailsClosedOnCorruption(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/bad.json", []byte("broken"), 0600))
	_, err := openJournal(dir)
	require.Error(t, err)
}

func TestJournalRejectsMissingOrDuplicateSequence(t *testing.T) {
	for _, tc := range []struct {
		name string
		seq  func(record, record) (uint64, uint64)
	}{
		{name: "zero sequence", seq: func(first, _ record) (uint64, uint64) { return 0, first.Sequence + 1 }},
		{name: "duplicate sequence", seq: func(first, _ record) (uint64, uint64) { return first.Sequence, first.Sequence }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			j, err := openJournal(dir)
			require.NoError(t, err)
			first, second := command(), command()
			require.NoError(t, j.enqueue(first))
			require.NoError(t, j.enqueue(second))
			a, b := j.records[first.ID], j.records[second.ID]
			a.Sequence, b.Sequence = tc.seq(a, b)
			require.NoError(t, j.update(a))
			require.NoError(t, j.update(b))
			require.NoError(t, j.root.Close())
			_, err = openJournal(dir)
			require.Error(t, err)
		})
	}
}

func TestJournalCapacityAndRetention(t *testing.T) {
	j, err := openJournal(t.TempDir())
	require.NoError(t, err)
	defer j.root.Close()
	for i := 0; i < maxPending; i++ {
		e := command()
		j.records[e.ID] = record{ID: e.ID, Command: &e}
	}
	require.Error(t, j.enqueue(command()))
	j.records = map[string]record{}
	e := command()
	require.NoError(t, j.enqueue(e))
	r, _ := j.next()
	old := time.Now().Add(-8 * 24 * time.Hour)
	r.Completed = &old
	require.NoError(t, j.update(r))
	require.NoError(t, j.prune())
	require.Empty(t, j.records)
}

func TestJournalPrunesExpiredIDBeforeDeduplication(t *testing.T) {
	j, err := openJournal(t.TempDir())
	require.NoError(t, err)
	defer j.root.Close()
	original := command()
	require.NoError(t, j.enqueue(original))
	r, ok := j.next()
	require.True(t, ok)
	event := deployment.NewRemovedEvent("web")
	event.RequestID = original.ID
	r.Event = &event
	expired := time.Now().Add(-receiptRetention - time.Second)
	r.Completed = &expired
	r.Command = nil
	require.NoError(t, j.update(r))

	reused := original
	reused.Data = json.RawMessage(`{"name":"other"}`)
	require.NoError(t, j.enqueue(reused))
	next, ok := j.next()
	require.True(t, ok)
	require.Equal(t, original.ID, next.ID)
	require.Equal(t, uint64(2), next.Sequence)
}

func TestJournalPoisonsAfterUncertainDirectorySyncFailure(t *testing.T) {
	j, err := openJournal(t.TempDir())
	require.NoError(t, err)
	defer j.root.Close()
	first := command()
	j.syncDir = func(*os.Root) error { return errors.New("injected directory sync failure") }
	require.Error(t, j.enqueue(first))
	require.Zero(t, j.sequence)
	require.Empty(t, j.records)

	firstPath := journalName(first.ID)
	persisted, err := j.root.ReadFile(firstPath)
	require.NoError(t, err)
	var diskRecord record
	require.NoError(t, json.Unmarshal(persisted, &diskRecord))
	require.Equal(t, uint64(1), diskRecord.Sequence)

	require.Error(t, j.enqueue(first))
	require.Error(t, j.update(diskRecord))
	second := command()
	require.Error(t, j.enqueue(second))
	_, err = j.root.ReadFile(journalName(second.ID))
	require.ErrorIs(t, err, os.ErrNotExist)
	current, err := j.root.ReadFile(firstPath)
	require.NoError(t, err)
	require.Equal(t, persisted, current)
}

type message struct {
	payload  []byte
	retained bool
	qos      byte
	ack      bool
}

func (m *message) Duplicate() bool   { return false }
func (m *message) Qos() byte         { return m.qos }
func (m *message) Retained() bool    { return m.retained }
func (m *message) Topic() string     { return "commands" }
func (m *message) MessageID() uint16 { return 1 }
func (m *message) Payload() []byte   { return m.payload }
func (m *message) Ack()              { m.ack = true }

func TestAckOnlyAfterPersistenceAndRejectRetained(t *testing.T) {
	l, err := NewListener("commands", t.TempDir(), &mock.Runtime{}, &mock.Publisher{})
	require.NoError(t, err)
	defer l.Close()
	b, _ := json.Marshal(command())
	m := &message{payload: b, qos: 1}
	l.HandleMessage(nil, m)
	require.True(t, m.ack)
	require.Len(t, l.journal.records, 1)
	m = &message{payload: b, qos: 1, retained: true}
	l.HandleMessage(nil, m)
	require.True(t, m.ack)
	require.Len(t, l.journal.records, 1)
	require.NoError(t, l.journal.root.Close())
	b, _ = json.Marshal(command())
	m = &message{payload: b, qos: 1}
	l.HandleMessage(nil, m)
	require.False(t, m.ack)
}

func TestCapacityDefersMessageUntilOutboxDrainsThenAcknowledges(t *testing.T) {
	rt := &v2RuntimeStub{}
	pub := &v2Publisher{events: make(chan deployment.Event, maxPending+1)}
	l, err := NewListener("commands", t.TempDir(), rt, pub)
	require.NoError(t, err)
	defer l.Close()
	for i := 0; i < maxPending-reservedQueryCapacity; i++ {
		e := v2Command(t, "remove", "web")
		require.NoError(t, l.journal.enqueue(e))
		event := deployment.Event{Version: 2, ID: uuid.New(), RequestID: e.ID, Timestamp: time.Now().UTC(), Action: "remove", Name: "web", Success: true, Code: "ok", Message: "Operation completed"}
		now := time.Now().UTC()
		require.NoError(t, l.journal.saveEvent(e.ID, event, &now, true))
	}
	e := v2Command(t, "remove", "later")
	b, err := json.Marshal(e)
	require.NoError(t, err)
	m := &message{payload: b, qos: 1}
	l.HandleMessage(nil, m)
	require.False(t, m.ack)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	pub.cancel = cancel
	pub.cancelRequestID = e.ID
	require.NoError(t, l.Start(ctx))
	require.True(t, m.ack)
	_, found := l.journal.find(e.ID)
	require.True(t, found)
}

func TestDeferredQueueOverflowStopsOnlyAfterQueuedMessagesPersist(t *testing.T) {
	pub := &v2Publisher{events: make(chan deployment.Event, 200)}
	l, err := NewListener("commands", t.TempDir(), &v2RuntimeStub{}, pub)
	require.NoError(t, err)
	defer l.Close()
	for i := 0; i < maxPending-reservedQueryCapacity; i++ {
		e := v2Command(t, "remove", "web")
		require.NoError(t, l.journal.enqueue(e))
		event := deployment.Event{Version: 2, ID: uuid.New(), RequestID: e.ID, Timestamp: time.Now().UTC(), Action: "remove", Name: "web", Success: true, Code: "ok", Message: "Operation completed"}
		now := time.Now().UTC()
		require.NoError(t, l.journal.saveEvent(e.ID, event, &now, true))
	}
	queued := make([]*message, deferredCapacity+1)
	ids := make([]string, deferredCapacity)
	for i := range queued {
		e := v2Command(t, "remove", "later")
		if i < deferredCapacity {
			ids[i] = e.ID
		}
		b, err := json.Marshal(e)
		require.NoError(t, err)
		queued[i] = &message{payload: b, qos: 1}
		l.HandleMessage(nil, queued[i])
	}
	for i := 0; i < deferredCapacity; i++ {
		require.False(t, queued[i].ack)
	}
	require.False(t, queued[deferredCapacity].ack)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.ErrorIs(t, l.Start(ctx), ErrDeferredOverflow)
	for i := 0; i < deferredCapacity; i++ {
		require.True(t, queued[i].ack)
		_, found := l.journal.find(ids[i])
		require.True(t, found)
	}
	require.False(t, queued[deferredCapacity].ack)
}

type resultPublisher struct {
	events chan deployment.Event
	fail   bool
	cancel context.CancelFunc
}

func (p *resultPublisher) Publish(_ context.Context, e deployment.Event) error {
	p.events <- e
	if p.cancel != nil {
		p.cancel()
	}
	if p.fail {
		return errors.New("broker offline")
	}
	return nil
}

func TestOutboxRetryAfterRestartDoesNotRepeatDeployment(t *testing.T) {
	dir := t.TempDir()
	rt := &mock.Runtime{}
	ctx, cancel := context.WithCancel(context.Background())
	p := &resultPublisher{events: make(chan deployment.Event, 2), fail: true, cancel: cancel}
	l, err := NewListener("commands", dir, rt, p)
	require.NoError(t, err)
	e := command()
	require.NoError(t, l.journal.enqueue(e))
	require.NoError(t, l.Start(ctx))
	require.Len(t, rt.RemoveCalls, 1)
	first := <-p.events
	require.Equal(t, e.ID, first.RequestID)
	require.NoError(t, l.Close())
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	p = &resultPublisher{events: make(chan deployment.Event, 2), cancel: cancel}
	l, err = NewListener("commands", dir, rt, p)
	require.NoError(t, err)
	defer l.Close()
	require.NoError(t, l.Start(ctx))
	second := <-p.events
	require.Equal(t, first.ID, second.ID)
	require.Len(t, rt.RemoveCalls, 1)
	_, ok := l.journal.next()
	require.False(t, ok)
}

func FuzzDecode(f *testing.F) {
	b, _ := json.Marshal(command())
	f.Add(b)
	f.Add([]byte("null"))
	f.Fuzz(func(t *testing.T, b []byte) { _, _ = Decode(b) })
}
