package mqtt

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/app"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/create"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/remove"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

type Listener struct {
	topic     string
	runtime   deployment.Runtime
	publisher deployment.Publisher
	journal   *journal
	wake      chan struct{}
	failures  chan error
}

func NewListener(topic, dir string, rt deployment.Runtime, pub deployment.Publisher) (*Listener, error) {
	j, err := openJournal(dir)
	if err != nil {
		return nil, err
	}
	return &Listener{topic: topic, runtime: rt, publisher: pub, journal: j, wake: make(chan struct{}, 1), failures: make(chan error, 1)}, nil
}

func (l *Listener) Close() error { return l.journal.root.Close() }
func (l *Listener) fail(err error) {
	select {
	case l.failures <- err:
	default:
	}
}

// Subscribe runs on every connection; deployment work never runs on Paho callbacks.
func (l *Listener) Subscribe(client paho.Client) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := Wait(ctx, client.Subscribe(l.topic, 1, l.HandleMessage)); err != nil {
		l.fail(fmt.Errorf("MQTT subscribe: %w", err))
		return
	}
	slog.Info("MQTT subscription ready", "topic", l.topic)
}

func (l *Listener) HandleMessage(_ paho.Client, msg paho.Message) {
	if msg.Retained() || msg.Qos() != 1 {
		slog.Warn("Rejected command: require non-retained QoS 1")
		msg.Ack()
		return
	}
	e, err := Decode(msg.Payload())
	if err != nil {
		slog.Warn("Rejected command", "error", err)
		msg.Ack()
		return
	}
	if err = l.journal.enqueue(e); err != nil {
		if errors.Is(err, ErrIDConflict) {
			slog.Warn("Rejected conflicting request ID", "request_id", e.ID)
			msg.Ack()
			return
		}
		l.fail(fmt.Errorf("persist command: %w", err))
		return
	}
	msg.Ack()
	select {
	case l.wake <- struct{}{}:
	default:
	}
}

func (l *Listener) Start(ctx context.Context) error {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}
		rec, ok := l.journal.next()
		if ok {
			if rec.Event == nil {
				event := l.execute(ctx, *rec.Command)
				event.RequestID = rec.ID
				rec.Event = &event
				if ctx.Err() != nil {
					return nil
				}
				if err := l.journal.update(rec); err != nil {
					return fmt.Errorf("persist result: %w", err)
				}
			}
			if err := l.publisher.Publish(ctx, *rec.Event); err == nil {
				now := time.Now().UTC()
				rec.Completed = &now
				rec.Command = nil
				if err := l.journal.update(rec); err != nil {
					return fmt.Errorf("persist receipt: %w", err)
				}
				slog.Info("Command completed", "request_id", rec.ID, "success", rec.Event.Success)
				continue
			} else if ctx.Err() == nil {
				slog.Warn("Event delivery pending; will retry", "request_id", rec.ID, "error", err)
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case err := <-l.failures:
			return err
		case <-l.wake:
		case <-ticker.C:
		}
	}
}

type capturePublisher struct{ event deployment.Event }

func (p *capturePublisher) Publish(_ context.Context, e deployment.Event) error {
	p.event = e
	return nil
}

func (l *Listener) execute(ctx context.Context, e Envelope) deployment.Event {
	p := &capturePublisher{}
	application := app.New(l.runtime, p)
	switch e.Type {
	case TypeCreate:
		var c create.Command
		_ = json.Unmarshal(e.Data, &c)
		if err := application.Deployment.Create.Handle(ctx, c); err != nil && p.event.Action == "" {
			return deployment.NewCreationFailedEvent(c.Name, err)
		}
	case TypeRemove:
		var c remove.Command
		_ = json.Unmarshal(e.Data, &c)
		if err := application.Deployment.Remove.Handle(ctx, c); err != nil && p.event.Action == "" {
			return deployment.NewRemovalFailedEvent(c.Name, err)
		}
	}
	return p.event
}
