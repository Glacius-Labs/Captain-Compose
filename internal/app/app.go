package app

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/application/event"
	"github.com/glacius-labs/captain-compose/internal/application/runtime"
	"github.com/glacius-labs/captain-compose/internal/infrastructure/docker"
)

type app struct {
	Runtime       runtime.Runtime
	CommandRouter *command.Router
	EventRouter   *event.Router
	listeners     []Listener
}

func NewApp(runtime runtime.Runtime) *app {
	return &app{
		Runtime:       runtime,
		CommandRouter: command.NewRouter(),
		EventRouter:   event.NewRouter(),
		listeners:     []Listener{},
	}
}

func NewDockerApp(composeDir string) (*app, error) {
	store, err := docker.NewStore(composeDir)
	if err != nil {
		return nil, err
	}

	runtime, err := docker.NewRuntime(store)
	if err != nil {
		return nil, err
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
