package docker

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glacius-labs/captain-compose/internal/control"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/stretchr/testify/require"
)

func TestV2CreatePersistsIntentAndResumesSameRequestAfterPartialApply(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	var calls []string
	upAttempts := 0
	r.run = func(_ context.Context, _ []byte, args ...string) ([]byte, error) {
		if args[len(args)-1] == "json" && args[len(args)-2] == "--format" {
			return []byte(canonical), nil
		}
		for _, arg := range args {
			if arg == "pull" {
				calls = append(calls, "pull")
			}
			if arg == "up" {
				calls = append(calls, "up")
			}
		}
		for _, arg := range args {
			if arg == "up" {
				upAttempts++
				if upAttempts == 1 {
					return nil, errors.New("could include private output")
				}
			}
		}
		return nil, nil
	}
	expectCreate := ""
	req := control.Request{Name: "web", Payload: []byte(validCompose), ExpectedRevision: &expectCreate, RequestID: "request-1"}
	_, err = r.Execute(context.Background(), "create", req)
	require.Equal(t, "apply_failed", control.ErrorCode(err))
	state, err := r.store.readState("web")
	require.NoError(t, err)
	require.Equal(t, "failed", state.Phase)
	require.NotEmpty(t, state.DesiredRevision)
	require.Equal(t, "request-1", state.LastRequestID)

	result, err := r.Execute(context.Background(), "create", req)
	require.NoError(t, err)
	require.Equal(t, "active", result.(control.DeploymentStatus).Phase)
	require.Equal(t, []string{"pull", "up", "pull", "up"}, calls)

	otherRequest := req
	otherRequest.RequestID = "request-2"
	_, err = r.Execute(context.Background(), "create", otherRequest)
	require.Equal(t, "revision_conflict", control.ErrorCode(err))
	state, err = r.store.readState("web")
	require.NoError(t, err)
	require.Equal(t, "active", state.Phase)
}

func TestV2PreflightFailureDoesNotChangeDesiredRevision(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	runCalls := 0
	r.run = func(_ context.Context, _ []byte, args ...string) ([]byte, error) {
		runCalls++
		if containsArg(args, "config") {
			return []byte(canonical), nil
		}
		if containsArg(args, "pull") {
			return nil, errors.New("private registry token must stay hidden")
		}
		t.Fatal("apply must not run when image preflight fails")
		return nil, nil
	}
	expected := ""
	_, err = r.Execute(context.Background(), "create", control.Request{Name: "web", Payload: []byte(validCompose), ExpectedRevision: &expected, RequestID: "preflight-1"})
	require.Equal(t, "image_preflight_failed", control.ErrorCode(err))
	_, err = r.store.readStateFile("web")
	require.ErrorIs(t, err, os.ErrNotExist)
	require.Equal(t, 2, runCalls, "only normalization and pull should run")
}

func TestV2StatusChecksDockerAndDoesNotWaitForMutationLock(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	err = r.store.saveState(deploymentState{Version: 2, Name: "web", Phase: "active", DesiredRevision: revisionOf([]byte(canonical)), Compose: []byte(canonical)})
	require.NoError(t, err)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.run = func(_ context.Context, _ []byte, args ...string) ([]byte, error) {
		if len(args) > 1 && args[0] == "info" {
			return []byte("27.0"), nil
		}
		if len(args) > 1 && args[0] == "compose" && args[1] == "version" {
			return []byte("2.30.0"), nil
		}
		if containsArg(args, "ps") {
			require.Contains(t, args, "--all")
			require.Contains(t, args, "--format")
			return []byte(`[{"Service":"web","State":"running","Health":"healthy","Image":"nginx:alpine"}]`), nil
		}
		t.Fatalf("unexpected command: %v", args)
		return nil, nil
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		value, execErr := r.Execute(context.Background(), "status", control.Request{})
		require.NoError(t, execErr)
		statuses := value.([]control.DeploymentStatus)
		require.Len(t, statuses, 1)
		require.Equal(t, "web", statuses[0].Name)
		require.False(t, statuses[0].Drift)
		require.Equal(t, "running", statuses[0].Services[0].State)
	}()
	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("status waited for the mutation mutex")
	}
}

func TestV2PlanContainsServiceChangesAndSafetyWarningsOnly(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	secret := "super-private-value"
	runCanonical := `{"services":{"web":{"image":"nginx:latest","environment":{"PASSWORD":"` + secret + `"},"volumes":["data:/data"]}}}`
	r.run = func(_ context.Context, _ []byte, args ...string) ([]byte, error) {
		if args[len(args)-1] == "json" {
			return []byte(runCanonical), nil
		}
		return nil, errors.New("unexpected operation")
	}
	value, err := r.Execute(context.Background(), "plan", control.Request{Name: "web", Payload: []byte(validCompose)})
	require.NoError(t, err)
	plan := value.(control.Plan)
	require.Equal(t, []string{"web"}, plan.Added)
	require.Contains(t, strings.Join(plan.Warnings, " "), "not pinned by digest")
	require.Contains(t, strings.Join(plan.Warnings, " "), "mounts")
	require.NotContains(t, strings.Join(plan.Warnings, " "), secret)
	require.NotContains(t, strings.Join(plan.Warnings, " "), "nginx")
}

func TestV2RevertRequiresDataRiskAndExpectedRevision(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	oldRevision := revisionOf([]byte("old"))
	err = r.store.saveState(deploymentState{Version: 2, Name: "web", Phase: "active", DesiredRevision: revisionOf([]byte(canonical)), Compose: []byte(canonical), History: []storedRevision{{Revision: oldRevision, Compose: []byte("old")}}})
	require.NoError(t, err)
	_, err = r.Execute(context.Background(), "revert", control.Request{Name: "web", Revision: oldRevision})
	require.Equal(t, "data_risk_acknowledgement_required", control.ErrorCode(err))
	_, err = r.Execute(context.Background(), "revert", control.Request{Name: "web", Revision: oldRevision, AllowDataRisk: true})
	require.Equal(t, "expected_revision_required", control.ErrorCode(err))
}

func TestV2RevertReappliesRetainedRevisionAndKeepsHistoryBound(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	oldCompose := []byte(`{"services":{"web":{"image":"nginx:1.25"}}}`)
	current := []byte(canonical)
	oldRevision, currentRevision := revisionOf(oldCompose), revisionOf(current)
	err = r.store.saveState(deploymentState{Version: 2, Name: "web", Phase: "active", DesiredRevision: currentRevision, SuccessfulRevision: currentRevision, Compose: current, History: []storedRevision{{Revision: oldRevision, Compose: oldCompose, Created: time.Now().Add(-time.Hour)}}})
	require.NoError(t, err)
	upCalls := 0
	r.run = func(_ context.Context, input []byte, args ...string) ([]byte, error) {
		if containsArg(args, "config") {
			return oldCompose, nil
		}
		if containsArg(args, "up") {
			upCalls++
		}
		return nil, nil
	}
	request := control.Request{Name: "web", ExpectedRevision: &currentRevision, Revision: oldRevision, AllowDataRisk: true, RequestID: "revert-1"}
	result, err := r.Execute(context.Background(), "revert", request)
	require.NoError(t, err)
	status := result.(control.DeploymentStatus)
	require.Equal(t, oldRevision, status.DesiredRevision)
	require.Equal(t, oldRevision, status.SuccessfulRevision)
	require.Len(t, status.Revisions, 2)
	require.Equal(t, 1, upCalls)
	_, err = r.Execute(context.Background(), "revert", request)
	require.NoError(t, err)
	require.Equal(t, 1, upCalls, "same completed request must be idempotent")

	history := make([]storedRevision, 0, historyLimit)
	for i := 0; i < historyLimit+5; i++ {
		compose := []byte(strings.Repeat("x", i+1))
		history = append(history, storedRevision{Revision: revisionOf(compose), Compose: compose, Created: time.Now().Add(time.Duration(i) * time.Second)})
	}
	bounded := boundHistory(history, history[0].Revision, true)
	require.LessOrEqual(t, len(bounded)+1, historyLimit)
	require.Contains(t, revisionsOf(bounded), history[0].Revision, "last successful revision remains recoverable")
}

func TestV2PlanRejectsStaleExpectedRevision(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	err = r.store.saveState(deploymentState{Version: 2, Name: "web", Phase: "active", DesiredRevision: revisionOf([]byte(canonical)), Compose: []byte(canonical)})
	require.NoError(t, err)
	r.run = func(_ context.Context, _ []byte, _ ...string) ([]byte, error) { return []byte(canonical), nil }
	stale := revisionOf([]byte("stale"))
	_, err = r.Execute(context.Background(), "plan", control.Request{Name: "web", Payload: []byte(validCompose), ExpectedRevision: &stale})
	require.Equal(t, "revision_conflict", control.ErrorCode(err))
}

func TestNonemptyExpectedRevisionRequiresExistingDeployment(t *testing.T) {
	stale := revisionOf([]byte(canonical))
	require.Equal(t, "revision_conflict", control.ErrorCode(checkExpected(&stale, false, stale)))

	for _, phase := range []string{"absent", "removed"} {
		t.Run(phase, func(t *testing.T) {
			r, err := NewRuntime(t.TempDir(), time.Second)
			require.NoError(t, err)
			defer r.Close()
			if phase == "removed" {
				err = r.store.saveState(deploymentState{Version: 2, Name: "web", Phase: "removed", SuccessfulRevision: stale, History: []storedRevision{{Revision: stale, Compose: []byte(canonical)}}})
				require.NoError(t, err)
			}
			r.run = func(_ context.Context, _ []byte, args ...string) ([]byte, error) {
				if containsArg(args, "config") {
					return []byte(canonical), nil
				}
				return nil, nil
			}
			_, err = r.Execute(context.Background(), "create", control.Request{Name: "web", Payload: []byte(validCompose), ExpectedRevision: &stale})
			require.Equal(t, "revision_conflict", control.ErrorCode(err))
			_, err = r.Execute(context.Background(), "plan", control.Request{Name: "web", Payload: []byte(validCompose), ExpectedRevision: &stale})
			require.Equal(t, "revision_conflict", control.ErrorCode(err))
		})
	}
}

func TestComposeVersionMinimum(t *testing.T) {
	for _, item := range []struct {
		version string
		ok      bool
	}{{"2.19.9", false}, {"2.20.0", true}, {"v2.33.1", true}, {"3.0.0", true}, {"unknown", false}} {
		require.Equal(t, item.ok, supportedComposeVersion(item.version), item.version)
	}
}

func TestRuntimeCheckEnforcesComposeMinimum(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	r.run = func(_ context.Context, _ []byte, args ...string) ([]byte, error) {
		if args[0] == "info" {
			return []byte("27.0"), nil
		}
		return []byte("2.19.9"), nil
	}
	require.ErrorContains(t, r.Check(context.Background()), "2.20 or newer")
	r.run = func(_ context.Context, _ []byte, args ...string) ([]byte, error) {
		if args[0] == "info" {
			return []byte("27.0"), nil
		}
		return []byte("v2.20.0"), nil
	}
	require.NoError(t, r.Check(context.Background()))
}

func TestDockerDiagnosticClassificationNeverReturnsStderr(t *testing.T) {
	private := "token super-private-value unauthorized"
	commandErr := &commandFailure{code: classifyDockerDiagnostic(private), cause: errors.New("exit status 1")}
	require.Equal(t, "registry_denied", control.ErrorCode(dockerError("apply_failed", commandErr)))
	require.NotContains(t, dockerError("apply_failed", commandErr).Error(), "super-private-value")
	require.Equal(t, "disk_full", classifyDockerDiagnostic("write /var/lib/docker: no space left on device"))
	require.Equal(t, "port_in_use", classifyDockerDiagnostic("port is already allocated"))
	require.Empty(t, classifyDockerDiagnostic("unknown diagnostic with secret abc"))
}

func TestStoreMigratesLegacyManifestToSingleValidatedRevision(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	_, err = r.store.save(deploymentName("legacy"), []byte(canonical))
	require.NoError(t, err)
	state, err := r.store.readState("legacy")
	require.NoError(t, err)
	require.Equal(t, "legacy", state.Phase)
	require.Equal(t, revisionOf([]byte(canonical)), state.DesiredRevision)
	require.NoError(t, r.store.saveState(state))
	_, err = r.store.root.Stat("legacy/compose.json")
	require.Error(t, err)
	stored, err := r.store.read("legacy")
	require.NoError(t, err)
	require.Equal(t, canonical, string(stored))
}

func TestStoreRejectsTamperedRevisionMetadata(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	state := deploymentState{Version: 2, Name: "web", Phase: "active", DesiredRevision: revisionOf([]byte(canonical)), Compose: []byte(canonical)}
	require.NoError(t, r.store.saveState(state))
	state.DesiredRevision = revisionOf([]byte("tampered"))
	require.Error(t, r.store.saveState(state))
}

func containsArg(args []string, needle string) bool {
	for _, arg := range args {
		if arg == needle {
			return true
		}
	}
	return false
}

func revisionsOf(history []storedRevision) []string {
	result := make([]string, 0, len(history))
	for _, item := range history {
		result = append(result, item.Revision)
	}
	return result
}

func deploymentName(name string) deployment.Deployment { return deployment.Deployment{Name: name} }
