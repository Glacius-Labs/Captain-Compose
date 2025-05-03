package mqtt

import (
	"context"
	"encoding/json"
	"fmt"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/application/event"
)

const topicBase = "captain-compose/v1/events"

type publisher struct {
	client mqtt.Client
}

func NewPublisher(client mqtt.Client) *publisher {
	return &publisher{client: client}
}

func (p *publisher) Publish(ctx context.Context, e event.Event) error {
	data, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("failed to marshal event %q: %w", e.Type(), err)
	}

	topic := fmt.Sprintf("%s/%s", topicBase, e.Type())

	token := p.client.Publish(topic, 1, false, data)
	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("failed to publish event %q to topic %q: %w", e.Type(), topic, token.Error())
	}

	return nil
}
