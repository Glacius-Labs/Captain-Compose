package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/observability"
)

// One pending token bounds memory even when the broker stops acknowledging.
// Heartbeats are non-retained, compatible with a broker disabling retained commands.
func heartbeat(ctx context.Context, client paho.Client, topic string, state *observability.State) {
	if topic == "" {
		return
	}
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	var pending paho.Token
	publish := func() {
		if pending != nil {
			select {
			case <-pending.Done():
				if pending.Error() != nil {
					slog.Warn("Heartbeat delivery failed")
				}
				pending = nil
			default:
				return
			}
		}
		if !client.IsConnectionOpen() {
			return
		}
		v := state.Snapshot()
		// Deployment detail remains on the local authenticated-access boundary.
		v.Deployments = nil
		payload, err := json.Marshal(v)
		if err != nil {
			return
		}
		pending = client.Publish(topic, 1, false, payload)
	}
	publish()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			publish()
		}
	}
}
