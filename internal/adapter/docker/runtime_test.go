package docker

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/stretchr/testify/require"
)

const validCompose = "services:\n  web:\n    image: nginx:alpine\n"
const canonical = `{"services":{"web":{"image":"nginx:alpine"}}}`

func TestDeployPersistsBeforeApplyAndRemoveSurvivesRestart(t *testing.T) {
	dir := t.TempDir()
	r, err := NewRuntime(dir, time.Second)
	require.NoError(t, err)
	r.run = func(_ context.Context, b []byte, args ...string) ([]byte, error) {
		if args[len(args)-2] == "--format" {
			return []byte(canonical), nil
		}
		stored, err := r.store.read("web")
		require.NoError(t, err)
		require.Equal(t, string(b), string(stored))
		require.Contains(t, args, "--wait")
		require.Contains(t, args, "cc-web")
		return nil, errors.New("partial deployment")
	}
	require.Error(t, r.Deploy(context.Background(), deployment.Deployment{Name: "web"}, []byte(validCompose)))
	require.NoError(t, r.Close())
	r, err = NewRuntime(dir, time.Second)
	require.NoError(t, err)
	defer r.Close()
	listed, err := r.List(context.Background())
	require.NoError(t, err)
	require.Equal(t, []deployment.Deployment{{Name: "web"}}, listed)
	calls := 0
	r.run = func(_ context.Context, b []byte, args ...string) ([]byte, error) {
		calls++
		require.JSONEq(t, canonical, string(b))
		require.Contains(t, args, "down")
		require.NotContains(t, args, "--volumes")
		return nil, nil
	}
	require.NoError(t, r.Remove(context.Background(), "web"))
	require.NoError(t, r.Remove(context.Background(), "web"))
	require.Equal(t, 1, calls)
}

func TestInvalidComposeDoesNotOverwriteState(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	_, err = r.store.save(deployment.Deployment{Name: "web"}, []byte(canonical))
	require.NoError(t, err)
	r.run = func(context.Context, []byte, ...string) ([]byte, error) { return nil, errors.New("invalid compose") }
	require.Error(t, r.Deploy(context.Background(), deployment.Deployment{Name: "web"}, []byte(validCompose)))
	b, err := r.store.read("web")
	require.NoError(t, err)
	require.Equal(t, canonical, string(b))
}

func TestRemoveFailureRetainsManifest(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	_, err = r.store.save(deployment.Deployment{Name: "web"}, []byte(canonical))
	require.NoError(t, err)
	r.run = func(context.Context, []byte, ...string) ([]byte, error) { return nil, errors.New("daemon unavailable") }
	require.Error(t, r.Remove(context.Background(), "web"))
	_, err = r.store.read("web")
	require.NoError(t, err)
}

func TestRejectUnsafeNamesAndNonSelfContainedCompose(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), time.Second)
	require.NoError(t, err)
	defer r.Close()
	r.run = func(context.Context, []byte, ...string) ([]byte, error) {
		t.Fatal("Docker must not run")
		return nil, nil
	}
	for _, name := range []string{"", "../escape", "--help", "UPPER", "a/b", strings.Repeat("a", 64)} {
		require.Error(t, r.Deploy(context.Background(), deployment.Deployment{Name: name}, []byte(validCompose)))
		require.Error(t, r.Remove(context.Background(), name))
	}
	for _, payload := range []string{"", "services: {}", "services: [oops]", validCompose + "---\nservices: {}", "include: /etc/secret\n" + validCompose, validCompose + "    env_file: /etc/secret\n", validCompose + "    build: .\n", validCompose + "    volumes: ['./data:/data']\n", validCompose + "secrets:\n  key:\n    file: /etc/key\n"} {
		require.Error(t, r.Deploy(context.Background(), deployment.Deployment{Name: "web"}, []byte(payload)), payload)
	}
}

func TestStoreConfinesSymlinks(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	if err := os.Symlink(outside, dir+"/escape"); err != nil {
		t.Skip("symlink permission unavailable")
	}
	s, err := newStore(dir)
	require.NoError(t, err)
	defer s.root.Close()
	_, err = s.save(deployment.Deployment{Name: "escape"}, []byte("secret"))
	require.Error(t, err)
	require.NoFileExists(t, outside+"/compose.json")
}

func TestOperationDeadline(t *testing.T) {
	r, err := NewRuntime(t.TempDir(), 10*time.Millisecond)
	require.NoError(t, err)
	defer r.Close()
	r.run = func(ctx context.Context, _ []byte, _ ...string) ([]byte, error) { <-ctx.Done(); return nil, ctx.Err() }
	require.ErrorIs(t, r.Deploy(context.Background(), deployment.Deployment{Name: "web"}, []byte(validCompose)), context.DeadlineExceeded)
}

func TestComposeOutputLimitAndLiteralDollarPreservation(t *testing.T) {
	b := &limitedBuffer{limit: 4}
	_, err := io.Copy(b, strings.NewReader("too much output"))
	require.Error(t, err)
	require.LessOrEqual(t, b.buffer.Len(), 4)
	value := map[string]any{"environment": map[string]any{"SECRET": "literal$HOME"}, "command": []any{"echo", "$VALUE"}}
	escaped := escapeComposeValues(value).(map[string]any)
	require.Equal(t, "literal$$HOME", escaped["environment"].(map[string]any)["SECRET"])
	require.Equal(t, "$$VALUE", escaped["command"].([]any)[1])
}
