package event

import "context"

type HandlerFunc func(ctx context.Context, evt Event) error

func (f HandlerFunc) Handle(ctx context.Context, evt Event) error {
	return f(ctx, evt)
}
