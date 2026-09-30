package mqtt

import (
	"context"
	paho "github.com/eclipse/paho.mqtt.golang"
)

func Wait(ctx context.Context, token paho.Token) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-token.Done():
		return token.Error()
	}
}
