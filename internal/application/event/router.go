package event

import (
	"context"
)

type Router struct {
	middleware Middleware
	handlers   []Handler
}

func NewRouter() *Router {
	return &Router{
		middleware: noOpMiddleware,
		handlers:   make([]Handler, 0),
	}
}

func (r *Router) Register(handler Handler) {
	r.handlers = append(r.handlers, handler)
}

func (r *Router) Use(middleware Middleware) {
	r.middleware = func(next Handler) Handler {
		return middleware(r.middleware(next))
	}
}

func (r *Router) Dispatch(ctx context.Context, event Event) {
	for _, handler := range r.handlers {
		h := r.middleware(handler)
		go func(handler Handler) {
			// Middleware is responsible for error handling. By default, errors are ignored.
			_ = handler.Handle(ctx, event)
		}(h)
	}
}

func noOpMiddleware(next Handler) Handler {
	return next
}
