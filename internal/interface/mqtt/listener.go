package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/application/command"
	createdeployment "github.com/glacius-labs/captain-compose/internal/interface/mqtt/deployment/create"
	removedeployment "github.com/glacius-labs/captain-compose/internal/interface/mqtt/deployment/remove"
	"github.com/glacius-labs/captain-compose/internal/interface/mqtt/message"
)

const topic = "captain-compose/v1/commands"

type listener struct {
	client   mqtt.Client
	decoders []message.Decoder
	router   command.Router
	logger   *slog.Logger
}

func NewListener(
	client mqtt.Client,
	router command.Router,
	logger *slog.Logger,
) *listener {
	decoders := []message.Decoder{
		createdeployment.NewDecoder(),
		removedeployment.NewDecoder(),
	}

	return &listener{
		client:   client,
		decoders: decoders,
		router:   router,
		logger:   logger,
	}
}

func (l *listener) Listen(ctx context.Context) error {
	l.logger.Info("MQTT listener starting", "topic", topic)

	token := l.client.Subscribe(topic, 1, func(_ mqtt.Client, msg mqtt.Message) {
		l.logger.Debug("Received MQTT message", "topic", msg.Topic())

		var env message.Envelope
		if err := json.Unmarshal(msg.Payload(), &env); err != nil {
			l.logger.Warn("Failed to decode envelope", "error", err)
			return
		}

		for _, decoder := range l.decoders {
			if decoder.CanDecode(env.Type) {
				cmd, err := decoder.Decode(&env)
				if err != nil {
					l.logger.Warn("Command decode failed", "type", env.Type, "error", err)
					return
				}

				if err := l.router.Dispatch(ctx, cmd); err != nil {
					l.logger.Error("Command dispatch failed", "type", cmd.Type(), "error", err)
				}
				return
			}
		}

		l.logger.Warn("Unhandled command type", "type", env.Type)
	})

	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to topic %q: %w", topic, token.Error())
	}

	<-ctx.Done()

	l.logger.Info("MQTT listener shutting down")
	l.client.Unsubscribe(topic)
	l.client.Disconnect(250)
	return nil
}
