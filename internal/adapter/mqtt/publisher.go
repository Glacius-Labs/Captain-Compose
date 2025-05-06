package mqtt

import (
	"context"
	"encoding/json"
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

type Publisher struct {
	topic  string
	client mqtt.Client
}

func NewPublisher(topic string, client mqtt.Client) *Publisher {
	return &Publisher{
		topic:  topic,
		client: client,
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

	return nil
}
