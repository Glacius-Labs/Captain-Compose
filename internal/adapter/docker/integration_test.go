//go:build integration

package docker

import (
	"context"
	"os"
	"os/exec"
	"testing"
	"time"

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
