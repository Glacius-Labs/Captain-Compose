package docker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"go.yaml.in/yaml/v3"
)

type runner func(context.Context, []byte, ...string) ([]byte, error)

type runtime struct {
	store   *store
	mu      sync.Mutex
	run     runner
	timeout time.Duration
}

func NewRuntime(dir string, timeout time.Duration) (*runtime, error) {
	s, err := newStore(dir)
	if err != nil {
		return nil, err
	}
	return &runtime{store: s, run: execute, timeout: timeout}, nil
}

func (r *runtime) Close() error { return r.store.root.Close() }

func (r *runtime) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if _, err := r.run(ctx, nil, "info", "--format", "{{.ServerVersion}}"); err != nil {
		return err
	}
	_, err := r.run(ctx, nil, "compose", "version", "--short")
	return err
}

// List reports this agent's persisted deployment intents, including failed applies.
func (r *runtime) List(ctx context.Context) ([]deployment.Deployment, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	dir, err := r.store.root.Open(".")
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, err
	}
	result := []deployment.Deployment{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := r.store.read(e.Name()); err == nil {
			result = append(result, deployment.Deployment{Name: e.Name()})
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (r *runtime) Deploy(ctx context.Context, d deployment.Deployment, payload []byte) error {
	if err := deployment.ValidateName(d.Name); err != nil {
		return err
	}
	if err := deployment.ValidatePayload(payload); err != nil {
		return err
	}
	if err := validateCompose(payload); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	args := composeArgs(d.Name)
	canonical, err := r.run(ctx, payload, append(args, "config", "--format", "json")...)
	if err != nil {
		return fmt.Errorf("validate compose: %w", err)
	}
	if !json.Valid(canonical) {
		return fmt.Errorf("compose returned invalid configuration")
	}
	// Compose interprets dollars again when reading its normalized output.
	// Escape string values so the second pass preserves the resolved configuration.
	var model any
	if err := json.Unmarshal(canonical, &model); err != nil {
		return err
	}
	canonical, err = json.Marshal(escapeComposeValues(model))
	if err != nil {
		return err
	}
	// Save before apply so failed/partial deployments remain removable after restart.
	if _, err := r.store.save(d, canonical); err != nil {
		return fmt.Errorf("persist compose: %w", err)
	}
	_, err = r.run(ctx, canonical, append(args, "up", "--detach", "--wait", "--remove-orphans", "--wait-timeout", fmt.Sprint(max(1, int(r.timeout.Seconds()))))...)
	return err
}

func (r *runtime) Remove(ctx context.Context, name string) error {
	if err := deployment.ValidateName(name); err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, r.timeout)
	defer cancel()
	payload, err := r.store.read(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if _, err := r.run(ctx, payload, append(composeArgs(name), "down", "--remove-orphans")...); err != nil {
		return err
	}
	p, _ := r.store.path(name)
	return r.store.root.Remove(p)
}

func composeArgs(name string) []string {
	return []string{"compose", "--env-file", os.DevNull, "--project-name", "cc-" + name, "--file", "-"}
}

// Remote requests are self-contained; no implicit host files.
func validateCompose(payload []byte) error {
	var doc map[string]any
	decoder := yaml.NewDecoder(bytes.NewReader(payload))
	if err := decoder.Decode(&doc); err != nil {
		return fmt.Errorf("invalid compose YAML")
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return fmt.Errorf("expected one compose document")
	}
	services, ok := doc["services"].(map[string]any)
	if !ok || len(services) == 0 {
		return fmt.Errorf("compose must define services")
	}
	if _, ok := doc["include"]; ok {
		return fmt.Errorf("compose include is not supported")
	}
	for _, raw := range services {
		svc, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid service")
		}
		if image, ok := svc["image"].(string); !ok || strings.TrimSpace(image) == "" {
			return fmt.Errorf("every service requires an image")
		}
		for _, key := range []string{"build", "extends", "env_file", "label_file", "develop", "profiles"} {
			if _, ok := svc[key]; ok {
				return fmt.Errorf("service %s is not supported in remote compose", key)
			}
		}
		if volumes, ok := svc["volumes"].([]any); ok {
			for _, v := range volumes {
				switch value := v.(type) {
				case string:
					parts := strings.Split(value, ":")
					if len(parts) > 1 && (strings.HasPrefix(parts[0], ".") || strings.HasPrefix(parts[0], "~")) {
						return fmt.Errorf("relative bind mounts are not supported")
					}
				case map[string]any:
					if value["type"] == "bind" {
						source, _ := value["source"].(string)
						if !strings.HasPrefix(source, "/") {
							return fmt.Errorf("bind source must be an absolute Linux path")
						}
					}
				}
			}
		}
	}
	for _, key := range []string{"configs", "secrets"} {
		items, _ := doc[key].(map[string]any)
		for _, raw := range items {
			item, _ := raw.(map[string]any)
			for _, field := range []string{"file", "environment"} {
				if _, ok := item[field]; ok {
					return fmt.Errorf("%s %s references are not supported", key, field)
				}
			}
		}
	}
	return nil
}

type limitedBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	if b.buffer.Len()+n > b.limit {
		return 0, fmt.Errorf("docker output limit exceeded")
	}
	_, err := b.buffer.Write(p)
	return n, err
}

func execute(ctx context.Context, input []byte, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stdin = bytes.NewReader(input)
	// Retain Docker connectivity and registry credentials, not agent MQTT secrets.
	for _, key := range []string{"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "WINDIR", "PROGRAMDATA", "PROGRAMFILES", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "SSH_AUTH_SOCK"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	var output = limitedBuffer{limit: 4 * deployment.MaxPayloadBytes}
	cmd.Stdout = &output
	cmd.Stderr = io.Discard // Compose diagnostics can contain secrets from the manifest.
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("docker operation interrupted: %w", ctx.Err())
		}
		return nil, fmt.Errorf("docker operation failed: %w (inspect the workload with docker compose)", err)
	}
	return output.buffer.Bytes(), nil
}

func escapeComposeValues(value any) any {
	switch v := value.(type) {
	case string:
		return strings.ReplaceAll(v, "$", "$$")
	case []any:
		for i, item := range v {
			v[i] = escapeComposeValues(item)
		}
	case map[string]any:
		for key, item := range v {
			v[key] = escapeComposeValues(item)
		}
	}
	return value
}
