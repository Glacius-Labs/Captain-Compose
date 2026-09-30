package mqtt

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
)

type Publisher struct {
	mu        sync.Mutex
	pending   mqtt.Token
	pendingID string
	topic     string
	client    mqtt.Client
}

func NewPublisher(topic string, client mqtt.Client) *Publisher {
	return &Publisher{
		topic:  topic,
		client: client,
	}
}

func (p *Publisher) Publish(ctx context.Context, event deployment.Event) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := ctx.Err(); err != nil {
		return err
	}
	if p.pending != nil && p.pendingID != event.ID.String() {
		return fmt.Errorf("previous event delivery is still pending")
	}
	if p.pending == nil && !p.client.IsConnectionOpen() {
		return fmt.Errorf("MQTT connection unavailable")
	}
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %w", err)
	}

	if p.pending == nil {
		p.pending = p.client.Publish(p.topic, 1, false, payload)
		p.pendingID = event.ID.String()
	}
	if err := Wait(ctx, p.pending); err != nil {
		select {
		case <-p.pending.Done():
			p.pending = nil
		default:
		}
		return fmt.Errorf("failed to publish event to topic %q: %w", p.topic, err)
	}
	p.pending = nil
	return nil
}
