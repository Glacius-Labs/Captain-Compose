package app

import (
	"context"
	"fmt"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/application/event"
	"github.com/glacius-labs/captain-compose/internal/core/deployment"
	"github.com/glacius-labs/captain-compose/internal/infrastructure/docker"
)

type app struct {
	Runtime       deployment.Runtime
	CommandRouter *command.Router
	EventRouter   *event.Router
	listeners     []Listener
}

func NewApp(runtime deployment.Runtime) *app {
	return &app{
		Runtime:       runtime,
		CommandRouter: command.NewRouter(),
		EventRouter:   event.NewRouter(),
		listeners:     []Listener{},
	}
}

func NewDockerApp(workingDir string) (*app, error) {
	runtime, err := docker.NewRuntime(workingDir)
	if err != nil {
		return nil, fmt.Errorf("failed to create docker runtime: %w", err)
	}

	return &app{
		Runtime:       runtime,
		CommandRouter: command.NewRouter(),
		EventRouter:   event.NewRouter(),
		listeners:     []Listener{},
	}, nil
}

func (a *app) RegisterCommandHandler(handler command.Handler) {
	if err := a.CommandRouter.Register(handler); err != nil {
		panic(err)
	}
}

func (a *app) RegisterEventHandler(handler event.Handler) {
	a.EventRouter.Register(handler)
}

func (a *app) RegisterEventMiddleware(middleware event.Middleware) {
	a.EventRouter.Use(middleware)
}

func (a *app) RegisterListener(listener Listener) {
	a.listeners = append(a.listeners, listener)
}

func (a *app) Run(ctx context.Context) {
	for _, listener := range a.listeners {
		go listener.Start(ctx)
	}
	<-ctx.Done()
}
