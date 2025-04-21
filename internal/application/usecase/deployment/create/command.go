package createdeployment

import (
	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/core/model"
)

const CommandType command.Type = "create.deployment"

type Command struct {
	Deployment model.Deployment
}

func (c *Command) Type() command.Type {
	return CommandType
}
