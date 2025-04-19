package commandhandler

import (
	"context"

	"github.com/glacius-labs/captain-compose/internal/application/command"
)

type CreateDeploymentHandler struct {
}

func (h *CreateDeploymentHandler) Handle(ctx context.Context, cmd command.CreateDeploymentCommand) error {
	panic("implement me")
}
