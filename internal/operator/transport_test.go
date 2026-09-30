package operator

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"github.com/glacius-labs/captain-compose/internal/control"
	"github.com/stretchr/testify/require"
)

type fakeMQTT struct {
	mu           sync.Mutex
	connected    bool
	subscribed   bool
	commands     [][]byte
	events       chan Event
	respondAfter int
	onPublish    func([]byte) Event
}

func newFakeMQTT(respondAfter int) *fakeMQTT {
	return &fakeMQTT{connected: true, events: make(chan Event, 4), respondAfter: respondAfter}
}
func (f *fakeMQTT) Subscribe(_ context.Context, topic string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.subscribed = topic == "cc/node/events"
	return nil
}
func (f *fakeMQTT) Publish(_ context.Context, topic string, payload []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if !f.subscribed || topic != "cc/node/commands" {
		return context.DeadlineExceeded
	}
	f.commands = append(f.commands, append([]byte(nil), payload...))
	if len(f.commands) >= f.respondAfter {
		var envelope Envelope
		_ = json.Unmarshal(payload, &envelope)
		if f.onPublish != nil {
			f.events <- f.onPublish(payload)
		} else {
			f.events <- Event{RequestID: "unrelated-request"}
			f.events <- Event{Version: 2, ID: "event-id", RequestID: envelope.ID, Action: envelope.Type, Success: true, Code: "ok"}
		}
	}
	return nil
}
func (f *fakeMQTT) Events() <-chan Event { return f.events }
func (f *fakeMQTT) Connected() bool      { return f.connected }

func TestFakeMQTTRoundTripSubscribesFirstCorrelatesAndRetriesSameEnvelope(t *testing.T) {
	env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events"}
	require.NoError(t, env.Validate())
	envelope := NewEnvelope("create", control.Request{Name: "web", Payload: []byte("services: {}")}, 0)
	fake := newFakeMQTT(2)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exchangeWithTransport(ctx, env, envelope, time.Millisecond, true, fake)
	require.NoError(t, err)
	require.True(t, out.Received)
	require.Equal(t, envelope.ID, out.Event.RequestID)
	require.Len(t, fake.commands, 2)
	require.Equal(t, fake.commands[0], fake.commands[1])
	var wire map[string]any
	require.NoError(t, json.Unmarshal(fake.commands[0], &wire))
	require.Equal(t, float64(2), wire["version"])
}

func TestFakeMQTTNoWaitMeansBrokerAcceptedWithoutAgentEvent(t *testing.T) {
	env := Environment{BrokerURL: "ssl://broker:8883", CommandTopic: "cc/node/commands", EventTopic: "cc/node/events"}
	fake := newFakeMQTT(99)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	out, err := exchangeWithTransport(ctx, env, NewEnvelope("remove", control.Request{Name: "web"}, 0), time.Second, false, fake)
	require.NoError(t, err)
	require.False(t, out.Received)
	require.Len(t, fake.commands, 1)
}
