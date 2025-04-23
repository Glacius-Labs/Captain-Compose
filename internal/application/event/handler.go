package event

import (
	"context"
)

type Handler interface {
	Name() string
	Handle(ctx context.Context, event Event) error
}

type HandlerFunc func(ctx context.Context, event Event) error

type HandlerFuncWrapper struct {
	name string
	fn   HandlerFunc
}

func NewHandlerFunc(name string, fn HandlerFunc) Handler {
	return &HandlerFuncWrapper{
		name: name,
		fn:   fn,
	}
}

func (h *HandlerFuncWrapper) Name() string {
	return h.name
}

func (h *HandlerFuncWrapper) Handle(ctx context.Context, event Event) error {
	return h.fn(ctx, event)
}
