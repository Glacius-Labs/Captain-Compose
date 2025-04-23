package createdeployment

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/application/event"
	"github.com/glacius-labs/captain-compose/internal/core/deployment"
)

type handler struct {
	runtime    deployment.Runtime
	dispatcher event.Dispatcher
}

func NewHandler(runtime deployment.Runtime, dispatcher event.Dispatcher) *handler {
	return &handler{runtime: runtime, dispatcher: dispatcher}
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
		h.dispatcher.Dispatch(ctx, deployment.NewCreationFailedEvent(createCmd.Name, createCmd.Payload, err))
		return err
	}

	h.dispatcher.Dispatch(ctx, deployment.NewCreatedEvent(createCmd.Name, createCmd.Payload))

	return nil
}
