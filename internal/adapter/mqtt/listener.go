package mqtt

import (
	"context"
	"encoding/json"
	"log/slog"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/app"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/create"
	"github.com/glacius-labs/captain-compose/internal/app/deployment/remove"
)

type Listener struct {
	topic  string
	client mqtt.Client
	app    *app.App
	logger *slog.Logger
}

func NewListener(topic string, client mqtt.Client, app *app.App, logger *slog.Logger) *Listener {
	return &Listener{
		topic:  topic,
		client: client,
		app:    app,
		logger: logger,
	}
}

func (l *Listener) Start(ctx context.Context) error {
	handler := func(_ mqtt.Client, msg mqtt.Message) {
		l.handleMessage(ctx, msg)
	}

	token := l.client.Subscribe(l.topic, 1, handler)
	if token.Wait() && token.Error() != nil {
		return token.Error()
	}

	l.logger.Info("MQTT subscription successful", "topic", l.topic)

	<-ctx.Done()

	l.logger.Info("Context cancelled, disconnecting from MQTT broker...")
	l.client.Disconnect(250)
	l.logger.Info("Disconnected from MQTT broker")

	return nil
}

func (l *Listener) handleMessage(ctx context.Context, msg mqtt.Message) {
	var env Envelope
	if err := json.Unmarshal(msg.Payload(), &env); err != nil {
		l.logger.Error("Failed to decode envelope", "error", err)
		return
	}

	switch env.Type {
	case TypeCreate:
		var cmd create.Command
		if err := json.Unmarshal(env.Data, &cmd); err != nil {
			l.logger.Error("Invalid create command", "error", err)
			return
		}
		if err := l.app.Deployment.Create.Handle(ctx, cmd); err != nil {
			l.logger.Error("Create command failed", "error", err, "name", cmd.Name)
		}
	case TypeRemove:
		var cmd remove.Command
		if err := json.Unmarshal(env.Data, &cmd); err != nil {
			l.logger.Error("Invalid remove command", "error", err)
			return
		}
		if err := l.app.Deployment.Remove.Handle(ctx, cmd); err != nil {
			l.logger.Error("Remove command failed", "error", err, "name", cmd.Name)
		}
	default:
		l.logger.Warn("Unknown command type", "type", env.Type)
	}
}
