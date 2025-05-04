package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

type Publisher struct {
	topic  string
	client mqtt.Client
	logger *slog.Logger
}

func NewPublisher(topic string, client mqtt.Client, logger *slog.Logger) *Publisher {
	return &Publisher{
		topic:  topic,
		client: client,
		logger: logger,
	}
}

func (p *Publisher) Publish(ctx context.Context, event deployment.Event) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	token := p.client.Publish(p.topic, 1, false, payload)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish event to topic %q: %w", p.topic, token.Error())
	}

	p.logger.Debug("Published event",
		"topic", p.topic,
		"id", event.ID,
		"action", event.Action,
		"name", event.Name,
		"success", event.Success,
		"message", event.Message,
	)

	return nil
}
