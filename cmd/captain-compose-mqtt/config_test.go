package main

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

const minimalConfig = `listener_topic: captain/node/commands
publisher_topic: captain/node/events
mqtt:
  broker_url: ssl://broker.example:8883
  client_id: node
`

func loadTestConfig(t *testing.T, raw string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	require.NoError(t, os.WriteFile(path, []byte(raw), 0600))
	return LoadConfig(path)
}

func TestConfigDefaultsAndSecretOverride(t *testing.T) {
	t.Setenv("CAPTAIN_COMPOSE_MQTT_PASSWORD", "secret")
	c, err := loadTestConfig(t, minimalConfig)
	require.NoError(t, err)
	require.Equal(t, "secret", c.MQTT.Password)
	require.Equal(t, "json", c.Log.Format)
	options, err := mqttOptions(c.MQTT)
	require.NoError(t, err)
	require.False(t, options.CleanSession)
	require.True(t, options.AutoAckDisabled)
	require.Equal(t, uint16(tls.VersionTLS12), options.TLSConfig.MinVersion)
}

func TestConfigurationRejectsTyposAndUnsafeSettings(t *testing.T) {
	for _, suffix := range []string{"unknown: true\n", "operation_timeout: 0s\n", "log:\n  level: typo\n", "---\nlog: {}\n", "  tls:\n    enable: true\n"} {
		_, err := loadTestConfig(t, minimalConfig+suffix)
		require.Error(t, err)
	}
	c, err := loadTestConfig(t, minimalConfig)
	require.NoError(t, err)
	c.MQTT.BrokerURL = "tcp://localhost:1883"
	require.Error(t, c.Validate())
	c.MQTT.AllowInsecure = true
	require.NoError(t, c.Validate())
	c.ListenerTopic = "captain/#"
	require.Error(t, c.Validate())
	c.ListenerTopic = c.PublisherTopic
	require.Error(t, c.Validate())
	_, err = newTLSConfig(TLSConfig{InsecureSkipVerify: true})
	require.Error(t, err)
	_, err = newTLSConfig(TLSConfig{ClientCertPath: "only-cert"})
	require.Error(t, err)
}
