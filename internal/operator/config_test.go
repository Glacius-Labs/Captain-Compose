package operator

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestLoadConfigNamedEnvironmentAndSecretSources(t *testing.T) {
	dir := t.TempDir()
	password := filepath.Join(dir, "password")
	require.NoError(t, os.WriteFile(password, []byte("test-secret\n"), 0600))
	path := filepath.Join(dir, "operator.yaml")
	content := "environments:\n  staging:\n    broker_url: ssl://mqtt.example.test:8883\n    command_topic: cc/staging/commands\n    event_topic: cc/staging/events\n    password_file: " + password + "\n    tls:\n      ca_cert_path: /tmp/ca.pem\n    timeout: 30s\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	cfg, err := LoadConfig(path)
	require.NoError(t, err)
	env := cfg.Environments["staging"]
	require.Equal(t, 30*time.Second, env.Timeout)
	require.NoError(t, env.ResolvePassword())
	require.Equal(t, "test-secret", env.Password)
	require.Empty(t, env.PasswordFile)
}

func TestEnvironmentRejectsUnsafeBrokerAndTopics(t *testing.T) {
	base := Environment{BrokerURL: "tcp://broker:1883", CommandTopic: "cc/n/commands", EventTopic: "cc/n/events"}
	require.ErrorContains(t, base.Validate(), "allow_insecure")
	base.AllowInsecure = true
	base.CommandTopic = "cc/+/commands"
	require.ErrorContains(t, base.Validate(), "exact MQTT topics")
	base.CommandTopic = "cc/n/commands"
	base.BrokerURL = "ssl://user:secret@broker:8883"
	require.ErrorContains(t, base.Validate(), "credentials belong in separate fields")
}

func TestConfigRejectsInlinePassword(t *testing.T) {
	path := filepath.Join(t.TempDir(), "operator.yaml")
	content := "environments:\n  staging:\n    broker_url: ssl://mqtt.example.test:8883\n    command_topic: cc/staging/commands\n    event_topic: cc/staging/events\n    password: unsafe-inline\n"
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	_, err := LoadConfig(path)
	require.Error(t, err)
}

func TestSplitFlagsAllowsOptionsAfterPositionals(t *testing.T) {
	flags, args := splitFlags([]string{"web", "compose.yaml", "--request-id", "uuid", "--wait=false", "--expected-revision", ""})
	require.Equal(t, []string{"--request-id", "uuid", "--wait=false", "--expected-revision", ""}, flags)
	require.Equal(t, []string{"web", "compose.yaml"}, args)
}
