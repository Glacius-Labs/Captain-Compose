package operator

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"os"
	"strings"
	"time"

	"go.yaml.in/yaml/v3"
)

type Config struct {
	Environments map[string]Environment `yaml:"environments"`
}
type Environment struct {
	BrokerURL     string        `yaml:"broker_url"`
	CommandTopic  string        `yaml:"command_topic"`
	EventTopic    string        `yaml:"event_topic"`
	Username      string        `yaml:"username"`
	Password      string        `yaml:"-"`
	PasswordFile  string        `yaml:"password_file,omitempty"`
	AllowInsecure bool          `yaml:"allow_insecure,omitempty"`
	TLS           TLSConfig     `yaml:"tls"`
	Timeout       time.Duration `yaml:"timeout,omitempty"`
	RetryInterval time.Duration `yaml:"retry_interval,omitempty"`
}
type TLSConfig struct {
	CACertPath     string `yaml:"ca_cert_path,omitempty"`
	ClientCertPath string `yaml:"client_cert_path,omitempty"`
	ClientKeyPath  string `yaml:"client_key_path,omitempty"`
}

func LoadConfig(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read operator config: %w", err)
	}
	d := yaml.NewDecoder(bytes.NewReader(b))
	d.KnownFields(true)
	var c Config
	if err := d.Decode(&c); err != nil {
		return nil, fmt.Errorf("parse operator config: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, fmt.Errorf("expected one configuration document")
	}
	if len(c.Environments) == 0 {
		return nil, fmt.Errorf("at least one environment is required")
	}
	for name, env := range c.Environments {
		if err := env.Validate(); err != nil {
			return nil, fmt.Errorf("environment %q: %w", name, err)
		}
	}
	return &c, nil
}
func (e Environment) Validate() error {
	u, err := url.Parse(e.BrokerURL)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid broker_url (credentials belong in separate fields)")
	}
	secure := u.Scheme == "ssl" || u.Scheme == "tls" || u.Scheme == "wss"
	if !secure && u.Scheme != "tcp" && u.Scheme != "ws" {
		return fmt.Errorf("unsupported broker URL scheme")
	}
	if !secure && !e.AllowInsecure {
		return fmt.Errorf("plaintext MQTT requires allow_insecure: true")
	}
	for _, topic := range []string{e.CommandTopic, e.EventTopic} {
		if topic == "" || len(topic) > 65535 || strings.ContainsAny(topic, "+#\x00") {
			return fmt.Errorf("command_topic and event_topic must be exact MQTT topics")
		}
	}
	if e.CommandTopic == e.EventTopic {
		return fmt.Errorf("command_topic and event_topic must differ")
	}
	if e.Password != "" && e.PasswordFile != "" {
		return fmt.Errorf("configure password or password_file, not both")
	}
	if (e.TLS.ClientCertPath == "") != (e.TLS.ClientKeyPath == "") {
		return fmt.Errorf("client certificate and key must be configured together")
	}
	if e.Timeout < 0 || e.Timeout > time.Hour || e.RetryInterval < 0 {
		return fmt.Errorf("timeout and retry_interval must be nonnegative; timeout at most 1h")
	}
	return nil
}
func (e *Environment) ResolvePassword() error {
	if value, ok := os.LookupEnv("CAPTAIN_COMPOSE_MQTT_PASSWORD"); ok {
		e.Password = value
		e.PasswordFile = ""
		return nil
	}
	if e.PasswordFile != "" {
		b, err := os.ReadFile(e.PasswordFile)
		if err != nil {
			return fmt.Errorf("read MQTT password file: %w", err)
		}
		e.Password = strings.TrimRight(string(b), "\r\n")
		e.PasswordFile = ""
	}
	return nil
}
