package event

import (
	"context"
	"log/slog"
)

type Router struct {
	handlers []Handler
	logger   *slog.Logger
}

func NewRouter(logger *slog.Logger) *Router {
	return &Router{
		handlers: make([]Handler, 0),
		logger:   logger,
	}
}

func (r *Router) RegisterFunc(name string, handlerFunc HandlerFunc) {
	wrapped := NewHandlerFunc(name, handlerFunc)
	r.RegisterHandler(wrapped)
}

func (r *Router) RegisterHandler(handler Handler) {
	r.handlers = append(r.handlers, handler)
	r.logger.Info("Registered handler", "handler_name", handler.Name())
}

func (r *Router) Dispatch(ctx context.Context, event Event) {
	r.logger.Info("Started dispatching event to handlers",
		"event_id", event.Identifier().String(),
		"event_type", event.Type(),
	)

	for _, h := range r.handlers {
		go func(h Handler) {
			if err := h.Handle(ctx, event); err != nil {
				r.logger.Error("Error handling event",
					"event_id", event.Identifier().String(),
					"event_type", event.Type(),
					"handler", h.Name(),
					"error", err,
				)
			} else {
				r.logger.Info("Event handled successfully",
					"event_id", event.Identifier().String(),
					"event_type", event.Type(),
					"handler", h.Name(),
				)
			}
		}(h)
	}
}
