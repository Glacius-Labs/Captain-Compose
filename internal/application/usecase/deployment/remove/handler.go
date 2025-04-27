package removedeployment

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/application/event"
	"github.com/glacius-labs/captain-compose/internal/core/deployment"
)

type handler struct {
	runtime   deployment.Runtime
	publisher event.Publisher
}

func NewHandler(runtime deployment.Runtime, publisher event.Publisher) *handler {
	return &handler{runtime: runtime, publisher: publisher}
}

func (h *handler) CommandType() command.Type {
	return CommandType
}

func (h *handler) Handle(ctx context.Context, cmd command.Command) error {
	removeCmd, ok := cmd.(*Command)
	if !ok {
		return command.NewCommandTypeMismatchError(h.CommandType(), cmd.Type())
	}

	if err := h.runtime.Remove(ctx, removeCmd.Name); err != nil {
		h.publisher.Publish(ctx, deployment.NewRemovalFailedEvent(removeCmd.Name, err))
		return err
	}

	h.publisher.Publish(ctx, deployment.NewRemovedEvent(removeCmd.Name))

	return nil
}
