package runtime

import (
	"context"
	"fmt"
	"sync"

	"github.com/glacius-labs/captain-compose/internal/core/deployment"
)

type mockRuntime struct {
	mu sync.Mutex

	Deployments map[string]deployment.Deployment

	FailDeploy bool
	FailRemove bool
}

func NewMockRuntime(failDeploy, failRemove bool) *mockRuntime {
	return &mockRuntime{
		Deployments: make(map[string]deployment.Deployment),
		FailDeploy:  failDeploy,
		FailRemove:  failRemove,
	}
}

func (m *mockRuntime) Deploy(ctx context.Context, deployment deployment.Deployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.FailDeploy {
		return fmt.Errorf("simulated deploy failure for %s", deployment.Name)
	}

	m.Deployments[deployment.Name] = deployment

	return nil
}

func (m *mockRuntime) Remove(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.FailRemove {
		return fmt.Errorf("simulated remove failure for %s", name)
	}

	delete(m.Deployments, name)

	return nil
}
