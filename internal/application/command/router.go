package command

import (
	"context"
	"fmt"
)

type Router struct {
	handlers map[Type]Handler
}

func NewRouter() *Router {
	return &Router{
		handlers: make(map[Type]Handler),
	}
}

func (r *Router) Register(handler Handler) error {
	cmdType := handler.CommandType()
	if _, exists := r.handlers[cmdType]; exists {
		return fmt.Errorf("handler for command type %q already registered", cmdType)
	}
	r.handlers[cmdType] = handler
	return nil
}

func (r *Router) Dispatch(ctx context.Context, cmd Command) error {
	handler, ok := r.handlers[cmd.Type()]
	if !ok {
		return fmt.Errorf("no handler registered for command type %q", cmd.Type())
	}
	return handler.Handle(ctx, cmd)
}
