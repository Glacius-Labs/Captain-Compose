package command

import (
	"context"
	"fmt"
	"log/slog"
)

type Router struct {
	handlers map[Type]Handler
	logger   *slog.Logger
}

func NewRouter(logger *slog.Logger) *Router {
	return &Router{
		handlers: make(map[Type]Handler),
		logger:   logger,
	}
}

func (r *Router) Register(handler Handler) error {
	cmdType := handler.CommandType()
	if _, exists := r.handlers[cmdType]; exists {
		return fmt.Errorf("handler for command type %q already registered", cmdType)
	}
	r.handlers[cmdType] = handler
	r.logger.Debug("Registered command handler", "command_type", cmdType)
	return nil
}

func (r *Router) Dispatch(ctx context.Context, cmd Command) error {
	r.logger.Debug("Dispatching command", "command_type", cmd.Type())

	handler, ok := r.handlers[cmd.Type()]
	if !ok {
		return fmt.Errorf("no handler registered for command type %q", cmd.Type())
	}

	return handler.Handle(ctx, cmd)
}
