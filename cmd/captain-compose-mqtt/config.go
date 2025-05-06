package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	MQTT           MQTTConfig `yaml:"mqtt"`
	Log            LogConfig  `yaml:"log"`
	ListenerTopic  string     `yaml:"listener_topic"`
	PublisherTopic string     `yaml:"publisher_topic"`
}

type MQTTConfig struct {
	BrokerURL string     `yaml:"broker_url"`
	ClientID  string     `yaml:"client_id"`
	Username  string     `yaml:"username"`
	Password  string     `yaml:"password"`
	TLS       *TLSConfig `yaml:"tls"` // optional TLS settings
}

type TLSConfig struct {
	Enable             bool   `yaml:"enable"`
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

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("unmarshal config: %w", err)
	}

	return &cfg, nil
}
