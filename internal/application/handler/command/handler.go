package commandhandler

import (
	"context"
)

type Handler[C command.Command] interface {
	Handle(ctx context.Context, cmd C) error
}
