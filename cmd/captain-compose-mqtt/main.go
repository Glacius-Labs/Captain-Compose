package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	paho "github.com/eclipse/paho.mqtt.golang"
	"github.com/glacius-labs/captain-compose/internal/adapter/docker"
	adapter "github.com/glacius-labs/captain-compose/internal/adapter/mqtt"
	"github.com/glacius-labs/captain-compose/internal/observability"
	"github.com/gofrs/flock"
)

var version = "dev"
var commit = "unknown"
var buildDate = "unknown"

func main() {
	if err := run(); err != nil {
		slog.Error("Captain Compose stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	configPath := flag.String("config", "config.yaml", "Path to YAML configuration")
	showVersion := flag.Bool("version", false, "Print version and exit")
	check := flag.Bool("check", false, "Validate configuration, TLS files and Docker access without subscribing")
	flag.Parse()
	if flag.NArg() != 0 {
		return fmt.Errorf("unexpected positional arguments")
	}
	if *showVersion {
		fmt.Printf("captain-compose-mqtt %s (%s, %s)\n", version, commit, buildDate)
		return nil
	}
	cfg, err := LoadConfig(*configPath)
	if err != nil {
		return err
	}
	closeLog, err := SetupLogger(cfg.Log)
	if err != nil {
		return err
	}
	defer closeLog()
	opts, err := mqttOptions(cfg.MQTT)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err = os.MkdirAll(cfg.StateDir, 0700); err != nil {
		return err
	}
	lock := flock.New(filepath.Join(cfg.StateDir, "agent.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !locked {
		return fmt.Errorf("another agent holds state_dir")
	}
	defer lock.Close()
	rt, err := docker.NewRuntime(filepath.Join(cfg.StateDir, "deployments"), cfg.OperationTimeout)
	if err != nil {
		return err
	}
	defer rt.Close()
	if err = rt.Check(ctx); err != nil {
		return fmt.Errorf("Docker readiness: %w", err)
	}
	if *check {
		slog.Info("Configuration, TLS and Docker checks passed")
		return nil
	}
	var listener *adapter.Listener
	if cfg.StatusTopic != "" {
		opts.SetWill(cfg.StatusTopic, `{"online":false}`, 1, false)
	}
	opts.SetOnConnectHandler(func(c paho.Client) { listener.Subscribe(c) })
	// Persistent broker sessions may deliver before SUBACK; register the route before connecting.
	client := paho.NewClient(opts)
	listener, err = adapter.NewListener(cfg.ListenerTopic, filepath.Join(cfg.StateDir, "journal"), rt, adapter.NewPublisher(cfg.PublisherTopic, client))
	if err != nil {
		return err
	}
	defer listener.Close()
	client.AddRoute(cfg.ListenerTopic, listener.HandleMessage)
	connectCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	err = adapter.Wait(connectCtx, client.Connect())
	cancel()
	defer client.Disconnect(250)
	if err != nil {
		return fmt.Errorf("MQTT connect failed (check broker, credentials and TLS)")
	}
	state := observability.New(version, func() observability.Queue {
		s := listener.Stats()
		return observability.Queue{Pending: s.Pending, Outbox: s.Outbox, Receipts: s.Receipts,
			OldestPendingSeconds: s.OldestPendingSeconds, OldestOutboxSeconds: s.OldestOutboxSeconds, LastCompleted: s.LastCompleted}
	}, client.IsConnectionOpen)
	backgroundCtx, cancelBackground := context.WithCancel(ctx)
	var background sync.WaitGroup
	defer func() { cancelBackground(); background.Wait() }()
	background.Add(1)
	go func() { defer background.Done(); state.Observe(backgroundCtx, rt, 15*time.Second) }()
	if cfg.StatusTopic != "" {
		background.Add(1)
		go func() { defer background.Done(); heartbeat(backgroundCtx, client, cfg.StatusTopic, state) }()
	}
	monitorFailure := make(chan error, 1)
	if cfg.MonitorListen != "" {
		address, done, err := state.Serve(backgroundCtx, cfg.MonitorListen)
		if err != nil {
			return fmt.Errorf("start monitoring: %w", err)
		}
		slog.Info("Local monitoring started", "address", address.String())
		background.Add(1)
		go func() {
			defer background.Done()
			if err := <-done; err != nil {
				monitorFailure <- err
				stop()
			}
		}()
	}
	slog.Info("Captain Compose started", "version", version, "client_id", cfg.MQTT.ClientID)
	err = listener.Start(ctx)
	select {
	case monitorErr := <-monitorFailure:
		return fmt.Errorf("monitoring stopped: %w", monitorErr)
	default:
	}
	return err
}
