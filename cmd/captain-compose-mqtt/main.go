package main

import (
	"fmt"
	"log/slog"

	"github.com/glacius-labs/captain-compose/internal/adapter/docker"
	"github.com/glacius-labs/captain-compose/internal/adapter/mqtt"
	"github.com/glacius-labs/captain-compose/internal/app"
)

func main() {
	cfg, err := LoadConfig("config.yaml")
	if err != nil {
		panic(fmt.Sprintf("failed to load config: %v", err))
	}

	if err := SetupLogger(cfg.Log); err != nil {
		panic(fmt.Sprintf("failed to setup logger: %v", err))
	}
	slog.Info("Logger initialized")

	rt, err := docker.NewRuntime()
	if err != nil {
		slog.Error("failed to create runtime", "error", err)
		return
	}

	mqttClient, err := SetupMQTT(cfg.MQTT)
	if err != nil {
		slog.Error("failed to connect to MQTT broker", "error", err)
		return
	}
	defer mqttClient.Disconnect(250)

	pub := mqtt.NewPublisher(cfg.PublisherTopic, mqttClient)
	application := app.New(rt, pub)

	listener := mqtt.NewListener(cfg.ListenerTopic, mqttClient, application)

	ctx := GracefulContext()

	if err := listener.Start(ctx); err != nil {
		slog.Error("Listener exited with error", "error", err)
	} else {
		slog.Info("Listener exited cleanly")
	}
}
