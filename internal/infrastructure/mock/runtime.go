package mock

import (
	"context"
	"fmt"
	"sync"

	"github.com/glacius-labs/captain-compose/internal/core/deployment"
)

type runtime struct {
	mu sync.Mutex

	Deployments map[string]deployment.Deployment

	FailDeploy bool
	FailRemove bool
}

func NewRuntime(failDeploy, failRemove bool) *runtime {
	return &runtime{
		Deployments: make(map[string]deployment.Deployment),
		FailDeploy:  failDeploy,
		FailRemove:  failRemove,
	}
}

func (m *runtime) List(ctx context.Context) ([]deployment.Deployment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	deployments := make([]deployment.Deployment, 0, len(m.Deployments))
	for _, deployment := range m.Deployments {
		deployments = append(deployments, deployment)
	}

	return deployments, nil
}

func (m *runtime) Get(ctx context.Context, name string) (deployment.Deployment, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	deployment, ok := m.Deployments[name]
	return deployment, ok
}

func (m *runtime) Deploy(ctx context.Context, deployment deployment.Deployment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.FailDeploy {
		return fmt.Errorf("simulated deploy failure for %s", deployment.Name)
	}

	m.Deployments[deployment.Name] = deployment

	return nil
}

func (m *runtime) Remove(ctx context.Context, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.FailRemove {
		return fmt.Errorf("simulated remove failure for %s", name)
	}

	delete(m.Deployments, name)

	return nil
}
