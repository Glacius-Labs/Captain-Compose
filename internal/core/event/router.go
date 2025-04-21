package event

import "context"

type router struct {
	middleware Middleware
	handlers   []Handler
}

func NewRouter() *router {
	return &router{
		middleware: noOpMiddleware,
		handlers:   make([]Handler, 0),
	}
}

func (r *router) Register(handler Handler) {
	r.handlers = append(r.handlers, handler)
}

func (r *router) Use(middleware Middleware) {
	r.middleware = func(next Handler) Handler {
		return middleware(r.middleware(next))
	}
}

func (r *router) Dispatch(ctx context.Context, evt Event) {
	for _, handler := range r.handlers {
		h := r.middleware(handler)
		go func(handler Handler) {
			// Middleware is responsible for error handling. By default, errors are ignored.
			_ = handler.Handle(ctx, evt)
		}(h)
	}
}
