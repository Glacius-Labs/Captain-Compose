package event

import "context"

type Handler interface {
	Handle(ctx context.Context, evt Event) error
}
