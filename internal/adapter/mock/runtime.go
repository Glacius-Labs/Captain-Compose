package mock

import (
	"context"
	"sync"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

type Runtime struct {
	mu sync.Mutex

	DeployFunc func(ctx context.Context, d deployment.Deployment, payload []byte) error
	RemoveFunc func(ctx context.Context, name string) error
	ListFunc   func(ctx context.Context) ([]deployment.Deployment, error)

	DeployCalls []DeployCall
	RemoveCalls []RemoveCall
	ListCalls   []ListCall
}

type DeployCall struct {
	Ctx     context.Context
	Deploy  deployment.Deployment
	Payload []byte
}

type RemoveCall struct {
	Ctx  context.Context
	Name string
}

type ListCall struct {
	Ctx context.Context
}

func (m *Runtime) Deploy(ctx context.Context, d deployment.Deployment, payload []byte) error {
	m.mu.Lock()
	m.DeployCalls = append(m.DeployCalls, DeployCall{ctx, d, payload})
	m.mu.Unlock()

	if m.DeployFunc != nil {
		return m.DeployFunc(ctx, d, payload)
	}
	return nil
}

func (m *Runtime) Remove(ctx context.Context, name string) error {
	m.mu.Lock()
	m.RemoveCalls = append(m.RemoveCalls, RemoveCall{ctx, name})
	m.mu.Unlock()

	if m.RemoveFunc != nil {
		return m.RemoveFunc(ctx, name)
	}
	return nil
}

func (m *Runtime) List(ctx context.Context) ([]deployment.Deployment, error) {
	m.mu.Lock()
	m.ListCalls = append(m.ListCalls, ListCall{ctx})
	m.mu.Unlock()

	if m.ListFunc != nil {
		return m.ListFunc(ctx)
	}
	return nil, nil
}
