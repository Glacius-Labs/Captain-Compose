package docker

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/glacius-labs/captain-compose/internal/control"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

// Execute implements the version 2 production operations. Query operations do
// not take the mutation mutex, so status remains available during a long apply.
func (r *runtime) Execute(ctx context.Context, action string, req control.Request) (any, error) {
	switch action {
	case "status":
		ctx, cancel := context.WithTimeout(ctx, minDuration(r.timeout, 15*time.Second))
		defer cancel()
		return r.status(ctx, req.Name)
	case "inspect":
		ctx, cancel := context.WithTimeout(ctx, minDuration(r.timeout, 15*time.Second))
		defer cancel()
		return r.inspect(ctx, req.Name)
	case "doctor":
		return r.doctor(ctx)
	case "plan":
		ctx, cancel := context.WithTimeout(ctx, minDuration(r.timeout, 15*time.Second))
		defer cancel()
		return r.plan(ctx, req)
	case "create", "remove", "revert":
		r.mu.Lock()
		defer r.mu.Unlock()
		ctx, cancel := context.WithTimeout(ctx, r.timeout)
		defer cancel()
		switch action {
		case "create":
			return r.create(ctx, req)
		case "remove":
			return r.removeV2(ctx, req)
		default:
			return r.revert(ctx, req)
		}
	default:
		return nil, control.Failure("unsupported_operation", "operation is not supported")
	}
}

func (r *runtime) create(ctx context.Context, req control.Request) (any, error) {
	if err := deployment.ValidateName(req.Name); err != nil {
		return nil, control.Failure("invalid_request", "deployment name is invalid")
	}
	if err := deployment.ValidatePayload(req.Payload); err != nil {
		return nil, control.Failure("invalid_request", "compose payload is invalid")
	}
	if err := validateCompose(req.Payload); err != nil {
		return nil, control.Failure("invalid_compose", err.Error())
	}
	canonical, _, err := r.normalize(ctx, req.Name, req.Payload)
	if err != nil {
		return nil, err
	}
	revision := revisionOf(canonical)
	state, stateErr := r.store.readState(req.Name)
	if errors.Is(stateErr, os.ErrNotExist) {
		stateErr = nil
	}
	if stateErr != nil {
		return nil, control.Failure("state_unavailable", "deployment state could not be read")
	}
	exists := state.Name != "" && state.Phase != "removed"
	resume := sameOperation(state, req, "create", revision)
	if requestIDConflict(state, req, "create", revision) {
		return nil, control.Failure("request_id_conflict", "request ID was already used for a different operation")
	}
	if !resume {
		if err := checkExpected(req.ExpectedRevision, exists, state.DesiredRevision); err != nil {
			return nil, err
		}
	}
	if resume && state.Phase == "active" {
		return statusFromState(state), nil
	}

	// Pull validates authenticated registry access and image availability before
	// desired state changes or any active service is replaced.
	if _, err := r.run(ctx, canonical, append(composeArgs(req.Name), "pull")...); err != nil {
		return nil, dockerError("image_preflight_failed", err)
	}
	if !resume {
		state = r.nextState(state, req.Name)
		if len(state.Compose) > 0 && state.DesiredRevision != revision {
			archiveCurrent(&state)
		}
		state.DesiredRevision, state.Compose = revision, canonical
		state.DesiredCreatedAt = time.Now().UTC()
		state.Phase = "applying"
		setOperation(&state, req, "create")
		state.UpdatedAt = time.Now().UTC()
		if err := r.persistDesired(state); err != nil {
			return nil, control.Failure("state_persist_failed", "desired deployment state could not be saved")
		}
	}
	args := append(composeArgs(req.Name), "up", "--detach", "--wait", "--remove-orphans", "--wait-timeout", fmt.Sprint(max(1, int(r.timeout.Seconds()))))
	if _, err := r.run(ctx, canonical, args...); err != nil {
		state.Phase, state.UpdatedAt = "failed", time.Now().UTC()
		if persistErr := r.store.saveState(state); persistErr != nil {
			return nil, control.Failure("state_persist_failed", "deployment failed and its failure state could not be saved")
		}
		return nil, dockerError("apply_failed", err)
	}
	state.SuccessfulRevision, state.Phase = revision, "active"
	state.UpdatedAt = time.Now().UTC()
	if err := r.store.saveState(state); err != nil {
		return nil, control.Failure("state_persist_failed", "deployment succeeded but its result could not be saved")
	}
	return statusFromState(state), nil
}

func (r *runtime) removeV2(ctx context.Context, req control.Request) (any, error) {
	if err := deployment.ValidateName(req.Name); err != nil {
		return nil, control.Failure("invalid_request", "deployment name is invalid")
	}
	if req.ExpectedRevision != nil && *req.ExpectedRevision == "" {
		return nil, control.Failure("invalid_request", "empty expected_revision is only supported for create")
	}
	state, err := r.store.readState(req.Name)
	if errors.Is(err, os.ErrNotExist) {
		if expected := req.ExpectedRevision; expected != nil && *expected != "" {
			return nil, control.Failure("revision_conflict", "deployment revision changed")
		}
		return control.DeploymentStatus{Name: req.Name, Phase: "removed", Services: []control.Service{}}, nil
	}
	if err != nil {
		return nil, control.Failure("state_unavailable", "deployment state could not be read")
	}
	resume := sameOperation(state, req, "remove", "")
	if requestIDConflict(state, req, "remove", "") {
		return nil, control.Failure("request_id_conflict", "request ID was already used for a different operation")
	}
	if state.Phase == "removed" && resume {
		return statusFromState(state), nil
	}
	if !resume {
		if state.Phase == "removed" {
			if expected := req.ExpectedRevision; expected != nil && *expected != "" {
				return nil, control.Failure("revision_conflict", "deployment revision changed")
			}
			return statusFromState(state), nil
		}
		if err := checkExpected(req.ExpectedRevision, true, state.DesiredRevision); err != nil {
			return nil, err
		}
		state.Phase = "removing"
		setOperation(&state, req, "remove")
		state.UpdatedAt = time.Now().UTC()
		if err := r.store.saveState(state); err != nil {
			return nil, control.Failure("state_persist_failed", "removal intent could not be saved")
		}
	}
	if len(state.Compose) != 0 {
		if _, err := r.run(ctx, state.Compose, append(composeArgs(req.Name), "down", "--remove-orphans")...); err != nil {
			state.Phase, state.UpdatedAt = "failed", time.Now().UTC()
			if persistErr := r.store.saveState(state); persistErr != nil {
				return nil, control.Failure("state_persist_failed", "removal failed and its failure state could not be saved")
			}
			return nil, dockerError("remove_failed", err)
		}
	}
	if len(state.Compose) > 0 {
		archiveCurrent(&state)
	}
	state.Phase, state.DesiredRevision, state.Compose = "removed", "", nil
	state.UpdatedAt = time.Now().UTC()
	if err := r.store.saveState(state); err != nil {
		return nil, control.Failure("state_persist_failed", "removal succeeded but its result could not be saved")
	}
	return statusFromState(state), nil
}

func (r *runtime) revert(ctx context.Context, req control.Request) (any, error) {
	if !req.AllowDataRisk {
		return nil, control.Failure("data_risk_acknowledgement_required", "revert requires explicit acknowledgement that application data is not rolled back")
	}
	if req.ExpectedRevision == nil {
		return nil, control.Failure("expected_revision_required", "revert requires the current expected revision")
	}
	if *req.ExpectedRevision == "" {
		return nil, control.Failure("invalid_request", "empty expected_revision is only supported for create")
	}
	if err := deployment.ValidateName(req.Name); err != nil {
		return nil, control.Failure("invalid_request", "deployment name is invalid")
	}
	state, err := r.store.readState(req.Name)
	if err != nil {
		return nil, control.Failure("not_found", "deployment history was not found")
	}
	resume := sameOperation(state, req, "revert", req.Revision)
	if requestIDConflict(state, req, "revert", req.Revision) {
		return nil, control.Failure("request_id_conflict", "request ID was already used for a different operation")
	}
	if resume && state.Phase == "active" {
		return statusFromState(state), nil
	}
	if !resume && *req.ExpectedRevision != state.DesiredRevision {
		return nil, control.Failure("revision_conflict", "deployment revision changed")
	}
	var target []byte
	if resume {
		target = state.Compose
	} else {
		if state.DesiredRevision == req.Revision {
			target = state.Compose
		}
		for _, item := range state.History {
			if item.Revision == req.Revision {
				target = item.Compose
			}
		}
	}
	if len(target) == 0 {
		return nil, control.Failure("revision_not_found", "requested revision is not in retained history")
	}
	canonical, _, err := r.normalize(ctx, req.Name, target)
	if err != nil {
		return nil, err
	}
	newRevision := revisionOf(canonical)
	if _, err := r.run(ctx, canonical, append(composeArgs(req.Name), "pull")...); err != nil {
		return nil, dockerError("image_preflight_failed", err)
	}
	if !resume {
		archiveCurrent(&state)
		state.DesiredRevision, state.Compose = newRevision, canonical
		state.DesiredCreatedAt = time.Now().UTC()
		state.Phase = "applying"
		setOperation(&state, req, "revert")
		state.UpdatedAt = time.Now().UTC()
		if err := r.persistDesired(state); err != nil {
			return nil, control.Failure("state_persist_failed", "revert intent could not be saved")
		}
	}
	args := append(composeArgs(req.Name), "up", "--detach", "--wait", "--remove-orphans", "--wait-timeout", fmt.Sprint(max(1, int(r.timeout.Seconds()))))
	if _, err := r.run(ctx, canonical, args...); err != nil {
		state.Phase = "failed"
		if persistErr := r.store.saveState(state); persistErr != nil {
			return nil, control.Failure("state_persist_failed", "revert failed and its failure state could not be saved")
		}
		return nil, dockerError("apply_failed", err)
	}
	state.SuccessfulRevision, state.Phase, state.UpdatedAt = newRevision, "active", time.Now().UTC()
	if err := r.store.saveState(state); err != nil {
		return nil, control.Failure("state_persist_failed", "revert succeeded but its result could not be saved")
	}
	return statusFromState(state), nil
}

func (r *runtime) plan(ctx context.Context, req control.Request) (any, error) {
	if err := deployment.ValidateName(req.Name); err != nil {
		return nil, control.Failure("invalid_request", "deployment name is invalid")
	}
	if err := deployment.ValidatePayload(req.Payload); err != nil {
		return nil, control.Failure("invalid_request", "compose payload is invalid")
	}
	if err := validateCompose(req.Payload); err != nil {
		return nil, control.Failure("invalid_compose", err.Error())
	}
	canonical, proposed, err := r.normalize(ctx, req.Name, req.Payload)
	if err != nil {
		return nil, err
	}
	state, err := r.store.readState(req.Name)
	if errors.Is(err, os.ErrNotExist) {
		err = nil
	}
	if err != nil {
		return nil, control.Failure("state_unavailable", "deployment state could not be read")
	}
	exists := state.Name != "" && state.Phase != "removed"
	if err := checkExpected(req.ExpectedRevision, exists, state.DesiredRevision); err != nil {
		return nil, err
	}
	var current map[string]any
	currentRevision := ""
	if state.Name != "" && state.Phase != "removed" {
		currentRevision = state.DesiredRevision
		_, current, err = parseServices(state.Compose)
		if err != nil {
			return nil, control.Failure("state_unavailable", "stored deployment configuration is invalid")
		}
	}
	added, changed, removed := diffServices(current, proposed)
	return control.Plan{Name: req.Name, CurrentRevision: currentRevision, Revision: revisionOf(canonical), Added: added, Changed: changed, Removed: removed, Warnings: planWarnings(canonical)}, nil
}

func (r *runtime) status(ctx context.Context, name string) (any, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if _, err := r.run(ctx, nil, "info", "--format", "{{.ServerVersion}}"); err != nil {
		return nil, dockerError("docker_unavailable", err)
	}
	composeVersion, err := r.run(ctx, nil, "compose", "version", "--short")
	if err != nil {
		return nil, dockerError("compose_unavailable", err)
	}
	versionWarning := ""
	if !supportedComposeVersion(string(composeVersion)) {
		versionWarning = "Docker Compose is below supported version 2.20"
	}
	if name != "" {
		if err := deployment.ValidateName(name); err != nil {
			return nil, control.Failure("invalid_request", "deployment name is invalid")
		}
		state, err := r.store.readState(name)
		if errors.Is(err, os.ErrNotExist) {
			return nil, control.Failure("not_found", "deployment was not found")
		}
		if err != nil {
			return nil, control.Failure("state_unavailable", "deployment state could not be read")
		}
		result, err := r.observeState(ctx, state)
		if err != nil {
			return nil, err
		}
		if versionWarning != "" {
			result.Warnings = append(result.Warnings, versionWarning)
		}
		return result, nil
	}
	root, err := r.store.root.Open(".")
	if err != nil {
		return nil, control.Failure("state_unavailable", "deployment state store could not be read")
	}
	entries, err := root.ReadDir(-1)
	_ = root.Close()
	if err != nil {
		return nil, control.Failure("state_unavailable", "deployment state store could not be read")
	}
	result := make([]control.DeploymentStatus, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		state, readErr := r.store.readState(entry.Name())
		if errors.Is(readErr, os.ErrNotExist) {
			continue
		}
		if readErr != nil {
			return nil, control.Failure("state_unavailable", "deployment state could not be read")
		}
		observed, observeErr := r.observeState(ctx, state)
		if observeErr != nil {
			return nil, observeErr
		}
		if versionWarning != "" {
			observed.Warnings = append(observed.Warnings, versionWarning)
		}
		result = append(result, observed)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

func (r *runtime) observeState(ctx context.Context, state deploymentState) (control.DeploymentStatus, error) {
	result := statusFromState(state)
	if len(state.Compose) != 0 && state.Phase != "removed" {
		services, err := r.observe(ctx, state.Name, state.Compose)
		if err != nil {
			return control.DeploymentStatus{}, dockerError("observation_failed", err)
		}
		result.Services = services
		result.ObservedAt = time.Now().UTC()
		result.Drift = observedDrift(state.Compose, services)
	}
	return result, nil
}

func (r *runtime) inspect(ctx context.Context, name string) (any, error) { return r.status(ctx, name) }

func (r *runtime) doctor(ctx context.Context) (any, error) {
	checks := make([]control.Check, 0, 4)
	ctx, cancel := context.WithTimeout(ctx, minDuration(r.timeout, 15*time.Second))
	defer cancel()
	_, err := r.run(ctx, nil, "info", "--format", "{{.ServerVersion}}")
	checks = append(checks, control.Check{Name: "docker_daemon", OK: err == nil, Message: checkMessage(err, "Docker daemon is reachable")})
	composeVersion, versionErr := r.run(ctx, nil, "compose", "version", "--short")
	versionOK := versionErr == nil && supportedComposeVersion(string(composeVersion))
	message := "Docker Compose 2.20 or newer is available"
	if versionErr != nil {
		message = "Docker Compose version could not be read"
	} else if !versionOK {
		message = "Docker Compose 2.20 or newer is required"
	}
	checks = append(checks, control.Check{Name: "docker_compose", OK: versionOK, Message: message})
	if err := ctx.Err(); err != nil {
		return checks, err
	}
	storeOK := r.storeWritable()
	storeMessage := "deployment state store is writable"
	if !storeOK {
		storeMessage = "deployment state store is unavailable or not writable"
	}
	checks = append(checks, control.Check{Name: "state_store", OK: storeOK, Message: storeMessage})
	registryOK, registryMessage := registryConfigCheck()
	checks = append(checks, control.Check{Name: "registry_config", OK: registryOK, Message: registryMessage})
	return checks, nil
}

func (r *runtime) storeWritable() bool {
	if r.store == nil || r.store.root == nil {
		return false
	}
	name := fmt.Sprintf(".doctor-check-%d.tmp", time.Now().UnixNano())
	f, err := r.store.root.OpenFile(name, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return false
	}
	_, writeErr := f.Write([]byte("ok"))
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	removeErr := r.store.root.Remove(name)
	if writeErr == nil {
		writeErr = closeErr
	}
	if writeErr == nil {
		writeErr = removeErr
	}
	if writeErr == nil {
		writeErr = syncDirectory(r.store.root, ".")
	}
	return writeErr == nil
}

func (r *runtime) normalize(ctx context.Context, name string, payload []byte) ([]byte, map[string]any, error) {
	canonical, err := r.run(ctx, payload, append(composeArgs(name), "config", "--format", "json")...)
	if err != nil {
		return nil, nil, dockerError("invalid_compose", err)
	}
	var model map[string]any
	if err := json.Unmarshal(canonical, &model); err != nil {
		return nil, nil, control.Failure("invalid_compose", "Docker Compose returned invalid configuration")
	}
	model = escapeComposeValues(model).(map[string]any)
	canonical, err = json.Marshal(model)
	if err != nil {
		return nil, nil, control.Failure("invalid_compose", "Compose configuration could not be normalized")
	}
	services, err := servicesFromCanonical(canonical)
	if err != nil {
		return nil, nil, control.Failure("invalid_compose", "Compose services could not be read")
	}
	return canonical, services, nil
}

func (r *runtime) observe(ctx context.Context, name string, compose []byte) ([]control.Service, error) {
	out, err := r.run(ctx, compose, append(composeArgs(name), "ps", "--all", "--format", "json")...)
	if err != nil {
		return nil, err
	}
	var rows []struct {
		Service string `json:"Service"`
		State   string `json:"State"`
		Health  string `json:"Health"`
		Image   string `json:"Image"`
	}
	if err := json.Unmarshal(out, &rows); err != nil {
		// Some Compose versions emit one JSON object per line.
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if line == "" {
				continue
			}
			var row struct {
				Service string `json:"Service"`
				State   string `json:"State"`
				Health  string `json:"Health"`
				Image   string `json:"Image"`
			}
			if err := json.Unmarshal([]byte(line), &row); err != nil {
				return nil, fmt.Errorf("invalid compose ps output")
			}
			rows = append(rows, row)
		}
	}
	services := make([]control.Service, 0, len(rows))
	for _, row := range rows {
		services = append(services, control.Service{Name: row.Service, State: row.State, Health: row.Health, Image: row.Image})
	}
	sort.Slice(services, func(i, j int) bool { return services[i].Name < services[j].Name })
	return services, nil
}

func (r *runtime) persistDesired(state deploymentState) error {
	return r.store.saveState(state)
}

func (r *runtime) nextState(state deploymentState, name string) deploymentState {
	if state.Name == "" {
		state = deploymentState{Version: 2, Name: name, Phase: "absent"}
	}
	return state
}

func checkExpected(expected *string, exists bool, current string) error {
	if expected == nil {
		return nil
	}
	if *expected == "" {
		if exists {
			return control.Failure("revision_conflict", "deployment already exists")
		}
		return nil
	}
	if !exists {
		return control.Failure("revision_conflict", "deployment revision changed")
	}
	if *expected != current {
		return control.Failure("revision_conflict", "deployment revision changed")
	}
	return nil
}

func sameOperation(state deploymentState, req control.Request, action, revision string) bool {
	return req.RequestID != "" && state.LastRequestID == req.RequestID && state.LastAction == action && sameExpectedRevision(state, req) && (revision == "" || state.DesiredRevision == revision)
}

func setOperation(state *deploymentState, req control.Request, action string) {
	state.LastRequestID, state.LastAction = req.RequestID, action
	state.HasExpectedRevision = req.ExpectedRevision != nil
	state.LastExpectedRevision = ""
	if req.ExpectedRevision != nil {
		state.LastExpectedRevision = *req.ExpectedRevision
	}
}

func sameExpectedRevision(state deploymentState, req control.Request) bool {
	if state.HasExpectedRevision != (req.ExpectedRevision != nil) {
		return false
	}
	return req.ExpectedRevision == nil || state.LastExpectedRevision == *req.ExpectedRevision
}

func requestIDConflict(state deploymentState, req control.Request, action, revision string) bool {
	return req.RequestID != "" && state.LastRequestID == req.RequestID && !sameOperation(state, req, action, revision)
}

func statusFromState(state deploymentState) control.DeploymentStatus {
	revisions := make([]control.Revision, 0, len(state.History)+1)
	seen := make(map[string]bool, len(state.History)+1)
	for _, item := range state.History {
		if !seen[item.Revision] {
			revisions = append(revisions, control.Revision{Revision: item.Revision, CreatedAt: item.Created})
			seen[item.Revision] = true
		}
	}
	if state.DesiredRevision != "" && !seen[state.DesiredRevision] {
		created := state.DesiredCreatedAt
		if created.IsZero() {
			created = state.UpdatedAt
		}
		revisions = append(revisions, control.Revision{Revision: state.DesiredRevision, CreatedAt: created})
	}
	sort.Slice(revisions, func(i, j int) bool {
		if revisions[i].CreatedAt.Equal(revisions[j].CreatedAt) {
			return revisions[i].Revision < revisions[j].Revision
		}
		return revisions[i].CreatedAt.Before(revisions[j].CreatedAt)
	})
	warnings := []string{}
	if len(state.Compose) != 0 {
		warnings = planWarnings(state.Compose)
	}
	return control.DeploymentStatus{Name: state.Name, DesiredRevision: state.DesiredRevision, SuccessfulRevision: state.SuccessfulRevision, LastRequestID: state.LastRequestID, Phase: state.Phase, ObservedAt: state.UpdatedAt, Services: []control.Service{}, Warnings: warnings, Revisions: revisions}
}

func archiveCurrent(state *deploymentState) {
	if len(state.Compose) == 0 || state.DesiredRevision == "" {
		return
	}
	for _, item := range state.History {
		if item.Revision == state.DesiredRevision {
			return
		}
	}
	created := state.DesiredCreatedAt
	if created.IsZero() {
		created = state.UpdatedAt
	}
	state.History = append(state.History, storedRevision{Revision: state.DesiredRevision, Compose: state.Compose, Created: created})
}

func dockerError(code string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return control.Failure("timeout", "Docker operation timed out; inspect deployment status before retrying")
	}
	if errors.Is(err, context.Canceled) {
		return control.Failure("cancelled", "Docker operation was cancelled; inspect deployment status before retrying")
	}
	var commandErr *commandFailure
	if errors.As(err, &commandErr) && commandErr.code != "" {
		switch commandErr.code {
		case "registry_denied":
			return control.Failure("registry_denied", "Registry denied image access; check credentials and repository permissions")
		case "image_unavailable":
			return control.Failure("image_unavailable", "An image tag or manifest is unavailable in the registry")
		case "registry_unreachable":
			if code == "image_preflight_failed" {
				return control.Failure("registry_unreachable", "The image registry could not be reached")
			}
			return control.Failure("endpoint_unreachable", "Docker or the image registry could not be reached")
		case "port_in_use":
			return control.Failure("port_in_use", "A requested host port is already in use")
		case "disk_full":
			return control.Failure("disk_full", "The Docker host has insufficient free disk space")
		}
	}
	return control.Failure(code, "Docker operation failed; inspect deployment status and daemon health")
}

func checkMessage(err error, ok string) string {
	if err == nil {
		return ok
	}
	return "check failed; inspect Docker daemon configuration"
}

var composeVersionPattern = regexp.MustCompile(`(?i)v?(\d+)\.(\d+)`)

func supportedComposeVersion(value string) bool {
	matches := composeVersionPattern.FindStringSubmatch(strings.TrimSpace(value))
	if len(matches) != 3 {
		return false
	}
	var major, minor int
	if _, err := fmt.Sscanf(matches[1], "%d", &major); err != nil {
		return false
	}
	if _, err := fmt.Sscanf(matches[2], "%d", &minor); err != nil {
		return false
	}
	return major >= 3 || (major == 2 && minor >= 20)
}

func registryConfigCheck() (bool, string) {
	configDir := os.Getenv("DOCKER_CONFIG")
	if configDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return false, "Docker registry configuration location is unavailable"
		}
		configDir = filepath.Join(home, ".docker")
	}
	contents, err := os.ReadFile(filepath.Join(configDir, "config.json"))
	if errors.Is(err, fs.ErrNotExist) {
		return true, "no registry credential file is configured; public images may be used"
	}
	if err != nil {
		return false, "Docker registry configuration exists but cannot be read"
	}
	if !json.Valid(contents) {
		return false, "Docker registry configuration is not valid JSON"
	}
	return true, "Docker registry configuration is readable"
}
func minDuration(a, b time.Duration) time.Duration {
	if a < b {
		return a
	}
	return b
}

func revisionOf(canonical []byte) string {
	sum := sha256.Sum256(canonical)
	return hex.EncodeToString(sum[:])
}

func parseServices(canonical []byte) ([]byte, map[string]any, error) {
	var root map[string]any
	if err := json.Unmarshal(canonical, &root); err != nil {
		return nil, nil, err
	}
	return canonical, serviceMap(root), nil
}
func servicesFromCanonical(canonical []byte) (map[string]any, error) {
	_, services, err := parseServices(canonical)
	if err != nil || len(services) == 0 {
		return nil, fmt.Errorf("services missing")
	}
	return services, nil
}
func serviceMap(root map[string]any) map[string]any {
	result, _ := root["services"].(map[string]any)
	if result == nil {
		return map[string]any{}
	}
	return result
}

func diffServices(current, proposed map[string]any) (added, changed, removed []string) {
	for name, value := range proposed {
		old, ok := current[name]
		if !ok {
			added = append(added, name)
			continue
		}
		a, _ := json.Marshal(old)
		b, _ := json.Marshal(value)
		if string(a) != string(b) {
			changed = append(changed, name)
		}
	}
	for name := range current {
		if _, ok := proposed[name]; !ok {
			removed = append(removed, name)
		}
	}
	sort.Strings(added)
	sort.Strings(changed)
	sort.Strings(removed)
	return
}

func planWarnings(canonical []byte) []string {
	var root map[string]any
	_ = json.Unmarshal(canonical, &root)
	services := serviceMap(root)
	warnings := map[string]bool{}
	for _, raw := range services {
		svc, _ := raw.(map[string]any)
		if _, ok := svc["healthcheck"]; !ok {
			warnings["one or more services have no healthcheck"] = true
		}
		image, _ := svc["image"].(string)
		if !strings.Contains(image, "@sha256:") {
			warnings["one or more images are not pinned by digest"] = true
		}
		if _, ok := svc["volumes"]; ok {
			warnings["one or more services use mounts; persistent data is outside revision rollback"] = true
		}
		memoryLimit, cpuLimit := false, false
		if deploy, ok := svc["deploy"].(map[string]any); ok {
			if resources, ok := deploy["resources"].(map[string]any); ok {
				if _, ok := resources["limits"]; ok {
					if limits, ok := resources["limits"].(map[string]any); ok {
						_, memoryLimit = limits["memory"]
						_, cpuLimit = limits["cpus"]
					}
				}
			}
		}
		if _, ok := svc["mem_limit"]; ok {
			memoryLimit = true
		}
		if _, ok := svc["cpus"]; ok {
			cpuLimit = true
		}
		if !memoryLimit {
			warnings["one or more services have no memory limit"] = true
		}
		if !cpuLimit {
			warnings["one or more services have no CPU limit"] = true
		}
	}
	result := make([]string, 0, len(warnings))
	for warning := range warnings {
		result = append(result, warning)
	}
	sort.Strings(result)
	return result
}

func observedDrift(canonical []byte, observed []control.Service) bool {
	desired, err := servicesFromCanonical(canonical)
	if err != nil {
		return true
	}
	actual := map[string]bool{}
	for _, svc := range observed {
		actual[svc.Name] = true
	}
	for name := range desired {
		if !actual[name] {
			return true
		}
	}
	for name := range actual {
		if _, ok := desired[name]; !ok {
			return true
		}
	}
	return false
}
