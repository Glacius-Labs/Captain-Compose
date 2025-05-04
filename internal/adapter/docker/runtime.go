package docker

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

type runtime struct {
	store   *store
	rwMutex sync.RWMutex
}

func NewRuntime() (*runtime, error) {
	return &runtime{
		store: newStore(),
	}, nil
}

func (r *runtime) List(ctx context.Context) ([]deployment.Deployment, error) {
	r.rwMutex.RLock()
	defer r.rwMutex.RUnlock()

	cmd := exec.CommandContext(ctx, "docker", "compose", "ls", "--format", "{{.Name}}")
	out, err := cmd.Output()
	if err != nil {
		return nil, wrapExecError(err, "docker compose ls")
	}

	var deployments []deployment.Deployment
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" {
			deployments = append(deployments, deployment.Deployment{Name: name})
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse docker compose ls output: %w", err)
	}

	return deployments, nil
}

func (r *runtime) Deploy(ctx context.Context, d deployment.Deployment, payload []byte) error {
	r.rwMutex.Lock()
	defer r.rwMutex.Unlock()

	composeFile, err := r.store.save(d, payload)
	if err != nil {
		return fmt.Errorf("failed to persist deployment: %w", err)
	}
	defer r.store.cleanup()

	cmd := exec.CommandContext(ctx, "docker", "compose", "-f", composeFile, "-p", d.Name, "up", "-d")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose up failed: %w\n%s", err, out)
	}

	return nil
}

func (r *runtime) Remove(ctx context.Context, name string) error {
	r.rwMutex.Lock()
	defer r.rwMutex.Unlock()

	cmd := exec.CommandContext(ctx, "docker", "compose", "-p", name, "down", "--volumes", "--remove-orphans")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("docker compose down failed: %w\nstderr: %s", err, out)
	}

	return nil
}

func wrapExecError(err error, action string) error {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("%s failed: %w\nstderr: %s", action, err, string(exitErr.Stderr))
	}
	return fmt.Errorf("%s failed: %w", action, err)
}
