package docker

import (
	"context"
	"fmt"
	"os/exec"
	"sync"

	"github.com/glacius-labs/captain-compose/internal/core/deployment"
)

type runtime struct {
	store *store
	mu    sync.Mutex
}

func NewRuntime(store *store) (*runtime, error) {
	if store == nil {
		panic("docker runtime requires a non-nil store")
	}

	r := &runtime{store: store}

	if err := r.cleanupDanglingProjects(context.Background()); err != nil {
		return nil, fmt.Errorf("runtime cleanup failed: %w", err)
	}

	return r, nil
}

func (r *runtime) Deploy(ctx context.Context, d deployment.Deployment) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := r.store.Add(d); err != nil {
		return fmt.Errorf("failed to persist deployment: %w", err)
	}

	composeFile := r.store.composeFilePath(d.Name)

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile, "-p", d.Name, "up", "-d")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up failed: %w\n%s", err, out)
	}

	return nil
}

func (r *runtime) Remove(ctx context.Context, name string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	cmd := exec.CommandContext(ctx, "docker", "compose", "-p", name, "down", "--volumes", "--remove-orphans")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down failed: %w\n%s", err, out)
	}

	if err := r.store.Remove(name); err != nil {
		return fmt.Errorf("failed to clean up deployment store: %w", err)
	}

	return nil
}
