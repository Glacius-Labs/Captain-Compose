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

func TestJournalFailsClosedOnCorruption(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/bad.json", []byte("broken"), 0600))
	_, err := openJournal(dir)
	require.Error(t, err)
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
