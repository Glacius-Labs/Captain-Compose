package mock

import (
	"context"
	"sync"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

type Runtime struct {
	mu          sync.Mutex
	DeployCalls []DeployCall
	RemoveCalls []RemoveCall
	DeployErr   error
	RemoveErr   error
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

func (m *Runtime) Deploy(ctx context.Context, d deployment.Deployment, payload []byte) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.DeployCalls = append(m.DeployCalls, DeployCall{ctx, d, payload})
	return m.DeployErr
}

func (m *Runtime) Remove(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.RemoveCalls = append(m.RemoveCalls, RemoveCall{ctx, name})
	return m.RemoveErr
}

func (m *Runtime) List(ctx context.Context) ([]deployment.Deployment, error) {
	return nil, nil
}
