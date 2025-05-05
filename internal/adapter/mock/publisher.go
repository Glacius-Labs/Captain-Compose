package mock

import (
	"context"
	"sync"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

type Publisher struct {
	mu    sync.Mutex
	Calls []PublishCall
	Err   error
}

type PublishCall struct {
	Ctx   context.Context
	Event deployment.Event
}

func (m *Publisher) Publish(ctx context.Context, event deployment.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.Calls = append(m.Calls, PublishCall{Ctx: ctx, Event: event})
	return m.Err
}

func (m *Publisher) CalledOnce() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.Calls) == 1
}
