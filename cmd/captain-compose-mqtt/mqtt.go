package main

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"log/slog"
	"os"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

func mqttOptions(cfg MQTTConfig) (*paho.ClientOptions, error) {
	tlsCfg, err := newTLSConfig(cfg.TLS)
	if err != nil {
		return nil, err
	}
	opts := paho.NewClientOptions().AddBroker(cfg.BrokerURL).SetClientID(cfg.ClientID).
		SetUsername(cfg.Username).SetPassword(cfg.Password).SetTLSConfig(tlsCfg).
		SetCleanSession(false).SetAutoAckDisabled(true).SetOrderMatters(true).
		SetConnectTimeout(10 * time.Second).SetWriteTimeout(10 * time.Second).
		SetKeepAlive(30 * time.Second).SetPingTimeout(10 * time.Second).
		SetAutoReconnect(true).SetMaxReconnectInterval(30 * time.Second).
		SetConnectionLostHandler(func(_ paho.Client, _ error) { slog.Warn("MQTT connection lost; reconnecting") })
	return opts, nil
}

func newTLSConfig(cfg TLSConfig) (*tls.Config, error) {
	if cfg.InsecureSkipVerify {
		return nil, fmt.Errorf("TLS certificate verification cannot be disabled")
	}
	if (cfg.ClientCertPath == "") != (cfg.ClientKeyPath == "") {
		return nil, fmt.Errorf("TLS certificate and key must be configured together")
	}
	t := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.CACertPath != "" {
		b, err := os.ReadFile(cfg.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(b) {
			return nil, fmt.Errorf("invalid CA certificate")
		}
		t.RootCAs = pool
	}
	if cfg.ClientCertPath != "" {
		cert, err := tls.LoadX509KeyPair(cfg.ClientCertPath, cfg.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load client certificate: %w", err)
		}
		t.Certificates = []tls.Certificate{cert}
	}
	return t, nil
}
