package mock

import (
	"context"
	"sync"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

type Publisher struct {
	mu          sync.Mutex
	PublishFunc func(ctx context.Context, event deployment.Event) error
	Calls       []PublishCall
}

type PublishCall struct {
	Ctx   context.Context
	Event deployment.Event
}

func (m *Publisher) Publish(ctx context.Context, event deployment.Event) error {
	m.mu.Lock()
	m.Calls = append(m.Calls, PublishCall{Ctx: ctx, Event: event})
	m.mu.Unlock()

	if m.PublishFunc != nil {
		return m.PublishFunc(ctx, event)
	}
	return nil
}

func (m *Publisher) CallsCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls)
}

func (m *Publisher) LastCall() (PublishCall, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.Calls) == 0 {
		return PublishCall{}, false
	}
	return m.Calls[len(m.Calls)-1], true
}
