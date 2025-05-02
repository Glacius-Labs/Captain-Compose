package mqtt

import (
	"context"
	"fmt"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
)

type Subscriber struct {
	client paho.Client
}

func NewSubscriberFromConfig(brokerURL, clientID string) (*Subscriber, error) {
	opts := paho.NewClientOptions().
		AddBroker(brokerURL).
		SetClientID(clientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(2 * time.Second)

	client := paho.NewClient(opts)

	if token := client.Connect(); token.Wait() && token.Error() != nil {
		return nil, fmt.Errorf("failed to connect to MQTT broker: %w", token.Error())
	}

	return &Subscriber{client: client}, nil
}

func (s *Subscriber) Subscribe(ctx context.Context, topic string, handler func(context.Context, []byte)) error {
	token := s.client.Subscribe(topic, 1, func(_ paho.Client, msg paho.Message) {
		go handler(ctx, msg.Payload())
	})

	if token.Wait() && token.Error() != nil {
		return fmt.Errorf("subscription to topic %q failed: %w", topic, token.Error())
	}

	return nil
}
