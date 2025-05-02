package mqtt

import (
	"context"
	"fmt"
	"log/slog"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/application/command"
)

type Decoder interface {
	Decode(data []byte) (command.Command, error)
}

type Listener struct {
	client  paho.Client
	topic   string
	decoder Decoder
	router  *command.Router
	logger  *slog.Logger
}

func NewListener(
	client paho.Client,
	topic string,
	decoder Decoder,
	router *command.Router,
	logger *slog.Logger,
) *Listener {
	return &Listener{
		client:  client,
		topic:   topic,
		decoder: decoder,
		router:  router,
		logger:  logger,
	}
}

func (l *Listener) Start(ctx context.Context) error {
	if token := l.client.Connect(); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	if token := l.client.Subscribe(l.topic, 1, l.handleMessage); token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to subscribe to topic %q: %w", l.topic, token.Error())
	}

	l.logger.Info("MQTT listener started", "topic", l.topic)

	<-ctx.Done()
	l.logger.Info("MQTT listener shutting down")
	l.client.Disconnect(250)

	return nil
}

func (l *Listener) handleMessage(_ paho.Client, msg paho.Message) {
	data := msg.Payload()

	cmd, err := l.decoder.Decode(data)
	if err != nil {
		l.logger.Warn("Failed to decode command", "error", err, "payload", string(data))
		return
	}

	if err := l.router.Dispatch(context.Background(), cmd); err != nil {
		l.logger.Error("Failed to dispatch command", "error", err, "command_type", cmd.Type())
	}
}
