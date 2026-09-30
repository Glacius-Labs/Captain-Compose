package mqtt

import (
	"context"
	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

type pendingToken struct{ done chan struct{} }

func (t *pendingToken) Done() <-chan struct{} { return t.done }
func (t *pendingToken) Wait() bool            { <-t.done; return true }
func (t *pendingToken) WaitTimeout(d time.Duration) bool {
	select {
	case <-t.done:
		return true
	case <-time.After(d):
		return false
	}
}
func (t *pendingToken) Error() error { return nil }

type publishClient struct {
	paho.Client
	calls int
	token *pendingToken
}

func (c *publishClient) IsConnectionOpen() bool { return true }
func (c *publishClient) Publish(_ string, _ byte, _ bool, _ any) paho.Token {
	c.calls++
	return c.token
}

func TestPublishRetriesWaitForSameInFlightToken(t *testing.T) {
	c := &publishClient{token: &pendingToken{done: make(chan struct{})}}
	p := NewPublisher("events", c)
	e := deployment.NewCreatedEvent("web")
	for i := 0; i < 2; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		require.ErrorIs(t, p.Publish(ctx, e), context.DeadlineExceeded)
		cancel()
	}
	require.Equal(t, 1, c.calls)
	close(c.token.done)
	require.NoError(t, p.Publish(context.Background(), e))
	require.Equal(t, 1, c.calls)
}
