//go:build integration

package docker

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/glacius-labs/captain-compose/internal/control"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestDockerLifecycle(t *testing.T) {
	if os.Getenv("CAPTAIN_INTEGRATION") != "1" {
		t.Skip("set CAPTAIN_INTEGRATION=1 to run isolated Docker tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	name := "test-" + uuid.NewString()[:8]
	volume := "cc-" + name + "_data"
	dir := t.TempDir()
	r, err := NewRuntime(dir, time.Minute)
	require.NoError(t, err)
	defer func() {
		_ = r.Remove(context.Background(), name)
		_ = r.Close()
		_ = exec.Command("docker", "volume", "rm", volume).Run()
	}()
	require.NoError(t, r.Check(ctx))
	payload := []byte("services:\n  app:\n    image: busybox:1.37.0\n    command: ['sh', '-c', 'touch /data/kept; sleep 3600']\n    volumes: ['data:/data']\nvolumes:\n  data: {}\n")
	require.NoError(t, r.Deploy(ctx, deployment.Deployment{Name: name}, payload))
	require.NoError(t, r.Deploy(ctx, deployment.Deployment{Name: name}, payload))
	require.NoError(t, r.Close())
	r, err = NewRuntime(dir, time.Minute)
	require.NoError(t, err)
	require.NoError(t, r.Remove(ctx, name))
	require.NoError(t, r.Remove(ctx, name))
	output, err := exec.CommandContext(ctx, "docker", "run", "--rm", "--mount", "type=volume,source="+volume+",target=/data", "busybox:1.37.0", "test", "-f", "/data/kept").CombinedOutput()
	require.NoError(t, err, string(output))
}

func TestDockerV2OperationsLifecycle(t *testing.T) {
	if os.Getenv("CAPTAIN_INTEGRATION") != "1" {
		t.Skip("set CAPTAIN_INTEGRATION=1 to run isolated Docker tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	name := "test-" + uuid.NewString()[:8]
	r, err := NewRuntime(t.TempDir(), time.Minute)
	require.NoError(t, err)
	defer func() {
		_ = r.Remove(context.Background(), name)
		_ = r.Close()
	}()
	require.NoError(t, r.Check(ctx))
	initial := []byte("services:\n  app:\n    image: busybox:1.37.0\n    command: ['sh', '-c', 'touch /data/kept; sleep 3600']\n    volumes: ['data:/data']\nvolumes:\n  data: {}\n")
	expectCreate := ""
	created, err := r.Execute(ctx, "create", control.Request{Name: name, Payload: initial, ExpectedRevision: &expectCreate, RequestID: uuid.NewString()})
	require.NoError(t, err)
	first := created.(control.DeploymentStatus)
	require.Equal(t, "active", first.Phase)
	require.NotEmpty(t, first.DesiredRevision)
	require.Equal(t, first.DesiredRevision, first.SuccessfulRevision)

	updated := []byte("services:\n  app:\n    image: busybox:1.37.0\n    command: ['sh', '-c', 'touch /data/updated; sleep 3600']\n    volumes: ['data:/data']\nvolumes:\n  data: {}\n")
	planValue, err := r.Execute(ctx, "plan", control.Request{Name: name, Payload: updated, ExpectedRevision: &first.DesiredRevision})
	require.NoError(t, err)
	require.Equal(t, []string{"app"}, planValue.(control.Plan).Changed)
	_, err = r.Execute(ctx, "create", control.Request{Name: name, Payload: updated, ExpectedRevision: &first.DesiredRevision, RequestID: uuid.NewString()})
	require.NoError(t, err)
	second, err := r.Execute(ctx, "status", control.Request{Name: name})
	require.NoError(t, err)
	current := second.(control.DeploymentStatus)
	require.Equal(t, "active", current.Phase)
	require.True(t, len(current.Services) > 0)
	require.False(t, current.Drift)
	require.GreaterOrEqual(t, len(current.Revisions), 2)

	reverted, err := r.Execute(ctx, "revert", control.Request{Name: name, ExpectedRevision: &current.DesiredRevision, Revision: first.DesiredRevision, AllowDataRisk: true, RequestID: uuid.NewString()})
	require.NoError(t, err)
	require.Equal(t, first.DesiredRevision, reverted.(control.DeploymentStatus).SuccessfulRevision)
	_, err = r.Execute(ctx, "remove", control.Request{Name: name, ExpectedRevision: &first.DesiredRevision, RequestID: uuid.NewString()})
	require.NoError(t, err)
	final, err := r.Execute(ctx, "status", control.Request{Name: name})
	require.NoError(t, err)
	require.Equal(t, "removed", final.(control.DeploymentStatus).Phase)
}
