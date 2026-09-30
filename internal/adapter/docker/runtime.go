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
	version, err := r.run(ctx, nil, "compose", "version", "--short")
	if err != nil {
		return err
	}
	if !supportedComposeVersion(string(version)) {
		return fmt.Errorf("Docker Compose 2.20 or newer is required")
	}
	return nil
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
	state, stateErr := r.store.readState(d.Name)
	if stateErr != nil && !errors.Is(stateErr, os.ErrNotExist) {
		return fmt.Errorf("read deployment state: %w", stateErr)
	}
	if state.Name == "" {
		state = deploymentState{Version: 2, Name: d.Name, Phase: "absent"}
	}
	revision := revisionOf(canonical)
	if len(state.Compose) > 0 && state.DesiredRevision != revision {
		archiveCurrent(&state)
	}
	state.DesiredRevision, state.Compose = revision, canonical
	state.DesiredCreatedAt = time.Now().UTC()
	state.Phase, state.LastAction, state.LastRequestID = "applying", "create", ""
	state.HasExpectedRevision, state.LastExpectedRevision = false, ""
	state.UpdatedAt = time.Now().UTC()
	if err := r.store.saveState(state); err != nil {
		return fmt.Errorf("persist compose: %w", err)
	}
	_, err = r.run(ctx, canonical, append(args, "up", "--detach", "--wait", "--remove-orphans", "--wait-timeout", fmt.Sprint(max(1, int(r.timeout.Seconds()))))...)
	if err != nil {
		state.Phase, state.UpdatedAt = "failed", time.Now().UTC()
		if persistErr := r.store.saveState(state); persistErr != nil {
			return fmt.Errorf("deployment failed and failure state could not be saved")
		}
		return err
	}
	state.SuccessfulRevision, state.Phase, state.UpdatedAt = revision, "active", time.Now().UTC()
	if err := r.store.saveState(state); err != nil {
		return fmt.Errorf("persist successful compose result: %w", err)
	}
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
	state, err := r.store.readState(name)
	if errors.Is(err, os.ErrNotExist) || (err == nil && state.Phase == "removed") {
		return nil
	}
	if err != nil {
		return err
	}
	state.Phase, state.LastAction, state.LastRequestID = "removing", "remove", ""
	state.HasExpectedRevision, state.LastExpectedRevision = false, ""
	state.UpdatedAt = time.Now().UTC()
	if err := r.store.saveState(state); err != nil {
		return fmt.Errorf("persist removal intent: %w", err)
	}
	if len(state.Compose) > 0 {
		if _, err := r.run(ctx, state.Compose, append(composeArgs(name), "down", "--remove-orphans")...); err != nil {
			state.Phase, state.UpdatedAt = "failed", time.Now().UTC()
			if persistErr := r.store.saveState(state); persistErr != nil {
				return fmt.Errorf("removal failed and failure state could not be saved")
			}
			return err
		}
	}
	if len(state.Compose) > 0 {
		archiveCurrent(&state)
	}
	state.Phase, state.DesiredRevision, state.Compose = "removed", "", nil
	state.UpdatedAt = time.Now().UTC()
	return r.store.saveState(state)
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

type diagnosticBuffer struct {
	buffer bytes.Buffer
	limit  int
}

func (b *diagnosticBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.limit - b.buffer.Len()
	if remaining > 0 {
		_, _ = b.buffer.Write(p[:min(len(p), remaining)])
	}
	return n, nil
}

type commandFailure struct {
	code  string
	cause error
}

func (e *commandFailure) Error() string { return "docker command failed" }
func (e *commandFailure) Unwrap() error { return e.cause }

func classifyDockerDiagnostic(stderr string) string {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "unauthorized"), strings.Contains(s, "authentication required"), strings.Contains(s, "denied: requested access"):
		return "registry_denied"
	case strings.Contains(s, "manifest unknown"), strings.Contains(s, "pull access denied"), strings.Contains(s, "not found: manifest"):
		return "image_unavailable"
	case strings.Contains(s, "no space left on device"):
		return "disk_full"
	case strings.Contains(s, "port is already allocated"), strings.Contains(s, "address already in use"):
		return "port_in_use"
	case strings.Contains(s, "no such host"), strings.Contains(s, "connection refused"), strings.Contains(s, "i/o timeout"), strings.Contains(s, "tls handshake timeout"):
		return "registry_unreachable"
	default:
		return ""
	}
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
	configureProcess(cmd)
	cmd.Stdin = bytes.NewReader(input)
	// Retain Docker connectivity and registry credentials, not agent MQTT secrets.
	for _, key := range []string{"PATH", "HOME", "USERPROFILE", "SYSTEMROOT", "WINDIR", "PROGRAMDATA", "PROGRAMFILES", "APPDATA", "LOCALAPPDATA", "TEMP", "TMP", "DOCKER_HOST", "DOCKER_CONTEXT", "DOCKER_CONFIG", "DOCKER_TLS_VERIFY", "DOCKER_CERT_PATH", "SSH_AUTH_SOCK"} {
		if value, ok := os.LookupEnv(key); ok {
			cmd.Env = append(cmd.Env, key+"="+value)
		}
	}
	var output = limitedBuffer{limit: 4 * deployment.MaxPayloadBytes}
	var diagnostic = diagnosticBuffer{limit: 8 * 1024}
	cmd.Stdout = &output
	cmd.Stderr = &diagnostic // Capture a bounded private buffer only for fixed classification.
	cmd.WaitDelay = 5 * time.Second
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("docker operation interrupted: %w", ctx.Err())
		}
		return nil, &commandFailure{code: classifyDockerDiagnostic(diagnostic.buffer.String()), cause: err}
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
