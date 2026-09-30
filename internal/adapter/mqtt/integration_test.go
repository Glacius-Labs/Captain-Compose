//go:build integration

package mqtt

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/adapter/mock"
	"github.com/glacius-labs/captain-compose/internal/domain/deployment"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestRealBrokerReconnectAndRecovery(t *testing.T) {
	if os.Getenv("CAPTAIN_INTEGRATION") != "1" {
		t.Skip("set CAPTAIN_INTEGRATION=1 to run isolated broker tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	name := "captain-mqtt-test-" + uuid.NewString()[:8]
	config := filepath.Join(t.TempDir(), "mosquitto.conf")
	require.NoError(t, os.WriteFile(config, []byte("listener 1883\nallow_anonymous true\nmax_packet_size 1600000\n"), 0644))
	docker := func(args ...string) string {
		t.Helper()
		out, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
		require.NoError(t, err, string(out))
		return strings.TrimSpace(string(out))
	}
	port, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	address := "tcp://" + port.Addr().String()
	binding := port.Addr().String() + ":1883"
	require.NoError(t, port.Close())
	docker("run", "--detach", "--name", name, "--publish", binding, "--mount", "type=bind,source="+config+",target=/mosquitto/config/mosquitto.conf,readonly", "eclipse-mosquitto:2.0.22")
	defer exec.Command("docker", "rm", "--force", name).Run()
	var listener *Listener
	connected := make(chan struct{}, 8)
	opts := paho.NewClientOptions().AddBroker(address).SetClientID(name).SetCleanSession(false).SetAutoAckDisabled(true).SetAutoReconnect(true).SetMaxReconnectInterval(time.Second).SetConnectTimeout(2 * time.Second)
	opts.SetOnConnectHandler(func(c paho.Client) { listener.Subscribe(c); connected <- struct{}{} })
	client := paho.NewClient(opts)
	listener, err = NewListener("commands", t.TempDir(), &mock.Runtime{}, NewPublisher("events", client))
	require.NoError(t, err)
	defer listener.Close()
	client.AddRoute("commands", listener.HandleMessage)
	require.Eventually(t, func() bool { return Wait(ctx, client.Connect()) == nil }, 10*time.Second, 500*time.Millisecond)
	defer client.Disconnect(250)
	done := make(chan error, 1)
	go func() { done <- listener.Start(ctx) }()
	defer func() { cancel(); require.NoError(t, <-done) }()
	select {
	case <-connected:
	case <-ctx.Done():
		t.Fatal("agent connect timeout")
	}
	events := make(chan deployment.Event, 8)
	controllerOpts := paho.NewClientOptions().AddBroker(address).SetClientID(name + "-controller").SetAutoReconnect(true).SetMaxReconnectInterval(time.Second)
	controllerOpts.SetOnConnectHandler(func(c paho.Client) {
		c.Subscribe("events", 1, func(_ paho.Client, m paho.Message) {
			var e deployment.Event
			if json.Unmarshal(m.Payload(), &e) == nil {
				events <- e
			}
		})
	})
	controller := paho.NewClient(controllerOpts)
	require.NoError(t, Wait(ctx, controller.Connect()))
	defer controller.Disconnect(250)
	require.NoError(t, Wait(ctx, controller.Subscribe("events", 1, func(_ paho.Client, m paho.Message) {
		var e deployment.Event
		if json.Unmarshal(m.Payload(), &e) == nil {
			events <- e
		}
	})))
	send := func() {
		t.Helper()
		e := command()
		b, _ := json.Marshal(e)
		require.NoError(t, Wait(ctx, controller.Publish("commands", 1, false, b)))
		select {
		case result := <-events:
			require.Equal(t, e.ID, result.RequestID)
			require.True(t, result.Success)
		case <-ctx.Done():
			t.Fatal("event timeout")
		}
	}
	send()
	docker("restart", name)
	select {
	case <-connected:
	case <-ctx.Done():
		t.Fatal("resubscribe timeout")
	}
	require.Eventually(t, controller.IsConnectionOpen, 10*time.Second, 100*time.Millisecond)
	require.NoError(t, Wait(ctx, controller.Subscribe("events", 1, func(_ paho.Client, m paho.Message) {
		var e deployment.Event
		if json.Unmarshal(m.Payload(), &e) == nil {
			events <- e
		}
	})))
	send()
}
