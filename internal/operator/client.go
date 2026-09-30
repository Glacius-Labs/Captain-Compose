package operator

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"os"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/control"
	"github.com/google/uuid"
)

type Envelope struct {
	Version   int             `json:"version"`
	ID        string          `json:"id"`
	Type      string          `json:"type"`
	ExpiresAt string          `json:"expires_at,omitempty"`
	Data      control.Request `json:"data"`
}
type Event struct {
	Version   int               `json:"version,omitempty"`
	ID        string            `json:"id"`
	RequestID string            `json:"request_id"`
	Timestamp time.Time         `json:"timestamp,omitempty"`
	Action    string            `json:"action"`
	Name      string            `json:"name,omitempty"`
	Success   bool              `json:"success"`
	Message   string            `json:"message,omitempty"`
	Labels    map[string]string `json:"labels,omitempty"`
	Code      string            `json:"code,omitempty"`
	Data      json.RawMessage   `json:"data,omitempty"`
}
type Outcome struct {
	Event    Event
	Received bool
}
type exchangeFunc func(context.Context, Environment, Envelope, time.Duration, bool) (Outcome, error)
type messageTransport interface {
	Subscribe(context.Context, string) error
	Publish(context.Context, string, []byte) error
	Events() <-chan Event
	Connected() bool
}

type pahoTransport struct {
	client paho.Client
	events chan Event
}

func (p *pahoTransport) Subscribe(ctx context.Context, topic string) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	return waitToken(ctx, p.client.Subscribe(topic, 1, nil))
}
func (p *pahoTransport) Publish(ctx context.Context, topic string, payload []byte) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := waitToken(ctx, p.client.Publish(topic, 1, false, payload)); err != nil {
		return &UnknownError{Message: "broker acknowledgement is uncertain after request publication"}
	}
	return nil
}
func (p *pahoTransport) Events() <-chan Event { return p.events }
func (p *pahoTransport) Connected() bool      { return p.client.IsConnectionOpen() }

type UnknownError struct{ Message string }

func (e *UnknownError) Error() string { return e.Message }

func NewEnvelope(kind string, req control.Request, expires time.Duration) Envelope {
	e := Envelope{Version: 2, ID: uuid.NewString(), Type: kind, Data: req}
	if expires > 0 {
		e.ExpiresAt = time.Now().Add(expires).UTC().Format(time.RFC3339)
	}
	return e
}
func NewClientOptions(env Environment, id string) (*paho.ClientOptions, error) {
	if err := env.ResolvePassword(); err != nil {
		return nil, err
	}
	tlsConfig, err := newTLSConfig(env.TLS)
	if err != nil {
		return nil, err
	}
	return paho.NewClientOptions().AddBroker(env.BrokerURL).SetClientID(id).SetUsername(env.Username).SetPassword(env.Password).
		SetTLSConfig(tlsConfig).SetCleanSession(true).SetAutoReconnect(false).SetConnectTimeout(10 * time.Second).
		SetWriteTimeout(10 * time.Second).SetKeepAlive(30 * time.Second).SetPingTimeout(10 * time.Second), nil
}
func newTLSConfig(c TLSConfig) (*tls.Config, error) {
	if (c.ClientCertPath == "") != (c.ClientKeyPath == "") {
		return nil, fmt.Errorf("client certificate and key must be configured together")
	}
	t := &tls.Config{MinVersion: tls.VersionTLS12}
	if c.CACertPath != "" {
		b, err := os.ReadFile(c.CACertPath)
		if err != nil {
			return nil, fmt.Errorf("read CA certificate: %w", err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(b) {
			return nil, fmt.Errorf("invalid CA certificate")
		}
		t.RootCAs = pool
	}
	if c.ClientCertPath != "" {
		cert, err := tls.LoadX509KeyPair(c.ClientCertPath, c.ClientKeyPath)
		if err != nil {
			return nil, fmt.Errorf("load client certificate")
		}
		t.Certificates = []tls.Certificate{cert}
	}
	return t, nil
}
func waitToken(ctx context.Context, token paho.Token) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-token.Done():
		if token.Error() != nil {
			return fmt.Errorf("MQTT operation failed")
		}
		return nil
	}
}

// Exchange subscribes before publishing. Retries reuse the byte-identical envelope and ID.
func Exchange(ctx context.Context, env Environment, envelope Envelope, retry time.Duration, waitResult bool) (Outcome, error) {
	var out Outcome
	if err := env.ResolvePassword(); err != nil {
		return out, err
	}
	if err := env.Validate(); err != nil {
		return out, err
	}
	if retry <= 0 {
		retry = 10 * time.Second
	}
	options, err := NewClientOptions(env, "cc-operator-"+uuid.NewString())
	if err != nil {
		return out, err
	}
	if envelope.ID == "" {
		return out, fmt.Errorf("request ID is required")
	}
	events := make(chan Event, 1)
	options.SetDefaultPublishHandler(func(_ paho.Client, msg paho.Message) {
		if len(msg.Payload()) > 1500*1024 {
			return
		}
		var e Event
		if json.Unmarshal(msg.Payload(), &e) == nil && e.RequestID == envelope.ID {
			select {
			case events <- e:
			default:
			}
		}
	})
	client := paho.NewClient(options)
	defer client.Disconnect(250)
	token := client.Connect()
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	if err := waitToken(connectCtx, token); err != nil {
		return out, fmt.Errorf("cannot connect to MQTT broker (check broker access and TLS configuration)")
	}
	return exchangeWithTransport(ctx, env, envelope, retry, waitResult, &pahoTransport{client: client, events: events})
}

func exchangeWithTransport(ctx context.Context, env Environment, envelope Envelope, retry time.Duration, waitResult bool, transport messageTransport) (Outcome, error) {
	var out Outcome
	if err := transport.Subscribe(ctx, env.EventTopic); err != nil {
		return out, fmt.Errorf("cannot subscribe to event topic (check broker ACLs)")
	}
	payload, err := json.Marshal(envelope)
	if err != nil {
		return out, fmt.Errorf("encode request")
	}
	deadline, hasDeadline := ctx.Deadline()
	nextPublish := time.Time{}
	published := false
	for {
		if nextPublish.IsZero() || time.Now().After(nextPublish) {
			if !transport.Connected() {
				if published {
					return out, &UnknownError{Message: "connection lost after request publication; operation result is unknown"}
				}
				return out, fmt.Errorf("MQTT connection lost")
			}
			if err = transport.Publish(ctx, env.CommandTopic, payload); err != nil {
				return out, err
			}
			published = true
			if !waitResult {
				return out, nil
			}
			nextPublish = time.Now().Add(retry)
		}
		wait := time.Until(nextPublish)
		if hasDeadline && time.Until(deadline) < wait {
			wait = time.Until(deadline)
		}
		if wait < 0 {
			wait = 0
		}
		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return out, ctx.Err()
		case ev := <-transport.Events():
			timer.Stop()
			if ev.RequestID != envelope.ID {
				continue
			}
			out.Event = ev
			out.Received = true
			return out, nil
		case <-timer.C:
			if hasDeadline && !time.Now().Before(deadline) {
				return out, context.DeadlineExceeded
			}
		}
	}
}
