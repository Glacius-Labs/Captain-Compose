package command

import (
	"context"
)

type Handler interface {
	CommandType() Type
	Handle(ctx context.Context, cmd Command) error
}
