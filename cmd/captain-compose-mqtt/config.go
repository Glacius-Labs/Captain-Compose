package main

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/glacius-labs/captain-compose/internal/observability"
	"go.yaml.in/yaml/v3"
)

type Config struct {
	StateDir         string        `yaml:"state_dir"`
	OperationTimeout time.Duration `yaml:"operation_timeout"`
	MQTT             MQTTConfig    `yaml:"mqtt"`
	Log              LogConfig     `yaml:"log"`
	ListenerTopic    string        `yaml:"listener_topic"`
	PublisherTopic   string        `yaml:"publisher_topic"`
	StatusTopic      string        `yaml:"status_topic"`
	MonitorListen    string        `yaml:"monitor_listen"`
}

type MQTTConfig struct {
	BrokerURL     string    `yaml:"broker_url"`
	ClientID      string    `yaml:"client_id"`
	Username      string    `yaml:"username"`
	Password      string    `yaml:"password"`
	PasswordFile  string    `yaml:"password_file"`
	AllowInsecure bool      `yaml:"allow_insecure"`
	TLS           TLSConfig `yaml:"tls"`
}

type TLSConfig struct {
	CACertPath         string `yaml:"ca_cert_path"`
	ClientCertPath     string `yaml:"client_cert_path"`
	ClientKeyPath      string `yaml:"client_key_path"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"` // dev only
}

type LogConfig struct {
	Level    string `yaml:"level"`     // "debug", "info", "warn", "error"
	Format   string `yaml:"format"`    // "text" or "json"
	FilePath string `yaml:"file_path"` // optional: "./captain-compose.log"
}

func LoadConfig(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config file: %w", err)
	}

	cfg := Config{StateDir: "./state", OperationTimeout: 5 * time.Minute, Log: LogConfig{Level: "info", Format: "json"}}
	decoder := yaml.NewDecoder(bytes.NewReader(raw))
	decoder.KnownFields(true)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := decoder.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("expected one configuration document")
	}
	if value, ok := os.LookupEnv("CAPTAIN_COMPOSE_MQTT_PASSWORD"); ok {
		cfg.MQTT.Password = value
		cfg.MQTT.PasswordFile = ""
	}
	if cfg.MQTT.PasswordFile != "" {
		if cfg.MQTT.Password != "" {
			return nil, fmt.Errorf("configure password or password_file, not both")
		}
		b, err := os.ReadFile(cfg.MQTT.PasswordFile)
		if err != nil {
			return nil, fmt.Errorf("read MQTT password file: %w", err)
		}
		cfg.MQTT.Password = strings.TrimRight(string(b), "\r\n")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (c Config) Validate() error {
	if err := observability.ValidateListen(c.MonitorListen); err != nil {
		return err
	}
	if c.StatusTopic != "" && (len(c.StatusTopic) > 65535 || strings.ContainsAny(c.StatusTopic, "+#\x00") || c.StatusTopic == c.ListenerTopic || c.StatusTopic == c.PublisherTopic) {
		return fmt.Errorf("status_topic must be a distinct exact MQTT topic")
	}
	if strings.TrimSpace(c.StateDir) == "" {
		return fmt.Errorf("state_dir is required")
	}
	if c.OperationTimeout < time.Second || c.OperationTimeout > time.Hour {
		return fmt.Errorf("operation_timeout must be between 1s and 1h")
	}
	if c.MQTT.ClientID == "" || strings.ContainsAny(c.MQTT.ClientID, "+#\x00") {
		return fmt.Errorf("mqtt.client_id must be a stable unique identifier")
	}
	for _, t := range []string{c.ListenerTopic, c.PublisherTopic} {
		if t == "" || len(t) > 65535 || strings.ContainsAny(t, "+#\x00") {
			return fmt.Errorf("topics must be nonempty exact MQTT topics without wildcards")
		}
	}
	if c.ListenerTopic == c.PublisherTopic {
		return fmt.Errorf("listener and publisher topics must differ")
	}
	u, err := url.Parse(c.MQTT.BrokerURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid mqtt.broker_url (credentials belong in separate fields)")
	}
	secure := u.Scheme == "ssl" || u.Scheme == "tls" || u.Scheme == "wss"
	if !secure && u.Scheme != "tcp" && u.Scheme != "ws" {
		return fmt.Errorf("unsupported MQTT URL scheme")
	}
	if !secure && !c.MQTT.AllowInsecure {
		return fmt.Errorf("plaintext MQTT requires explicit allow_insecure: true")
	}
	if c.MQTT.TLS.InsecureSkipVerify {
		return fmt.Errorf("TLS certificate verification cannot be disabled")
	}
	if (c.MQTT.TLS.ClientCertPath == "") != (c.MQTT.TLS.ClientKeyPath == "") {
		return fmt.Errorf("TLS client certificate and key must be configured together")
	}
	switch c.Log.Level {
	case "debug", "info", "warn", "error":
	default:
		return fmt.Errorf("invalid log.level")
	}
	switch c.Log.Format {
	case "json", "text":
	default:
		return fmt.Errorf("invalid log.format")
	}
	return nil
}
