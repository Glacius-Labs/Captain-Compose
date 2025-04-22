package docker

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

func (r *runtime) cleanupDanglingProjects(ctx context.Context) error {
	active, err := r.listActiveProjects(ctx)
	if err != nil {
		return err
	}

	activeSet := make(map[string]struct{}, len(active))
	for _, name := range active {
		activeSet[name] = struct{}{}
	}

	for _, d := range r.store.All() {
		if _, ok := activeSet[d.Name]; !ok {
			if err := r.store.Remove(d.Name); err != nil {
				return fmt.Errorf("failed to remove dangling project %s: %w", d.Name, err)
			}
		}
	}

	return nil
}

func (r *runtime) listActiveProjects(ctx context.Context) ([]string, error) {
	cmd := exec.CommandContext(ctx, "docker", "compose", "ls", "--format", "{{.Name}}")
	out, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("docker compose ls failed: %w\nstderr: %s", err, string(exitErr.Stderr))
		}
		return nil, fmt.Errorf("docker compose ls failed: %w", err)
	}

	var names []string
	scanner := bufio.NewScanner(bytes.NewReader(out))
	for scanner.Scan() {
		name := strings.TrimSpace(scanner.Text())
		if name != "" {
			names = append(names, name)
		}
	}
	return names, nil
}
