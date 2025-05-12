//go:build integration

package docker

import (
	"context"
	"os"
	"testing"

	"github.com/glacius-labs/captain-compose/internal/core/deployment"
	"github.com/stretchr/testify/require"
)

func TestDockerRuntime(t *testing.T) {
	if os.Getenv("CI_DOCKER_TEST") != "1" {
		t.Skip("skipping docker integration tests unless CI_DOCKER_TEST=1 is set")
	}

	ctx := context.Background()

	rt, err := NewRuntime()
	require.NoError(t, err)

	d := deployment.Deployment{Name: "test-integration-deploy"}
	payload := []byte(`
version: "3"
services:
  hello:
    image: busybox
    command: ["echo", "hello world"]
`)

	t.Run("Deploy", func(t *testing.T) {
		err := rt.Deploy(ctx, d, payload)
		require.NoError(t, err)
	})

	t.Run("List", func(t *testing.T) {
		deployments, err := rt.List(ctx)
		require.NoError(t, err)
		var found bool
		for _, dep := range deployments {
			if dep.Name == d.Name {
				found = true
				break
			}
		}
		require.True(t, found, "deployment not found in list")
	})

	t.Run("Remove", func(t *testing.T) {
		err := rt.Remove(ctx, d.Name)
		require.NoError(t, err)
	})
}
