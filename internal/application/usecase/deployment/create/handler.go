package createdeployment

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
	createCmd, ok := cmd.(*Command)
	if !ok {
		return command.NewCommandTypeMismatchError(h.CommandType(), cmd.Type())
	}

	d := deployment.Deployment{
		Name: createCmd.Name,
	}

	if err := h.runtime.Deploy(ctx, d, createCmd.Payload); err != nil {
		h.publisher.Publish(ctx, deployment.NewCreationFailedEvent(createCmd.Name, createCmd.Payload, err))
		return err
	}

	h.publisher.Publish(ctx, deployment.NewCreatedEvent(createCmd.Name, createCmd.Payload))

	return nil
}
