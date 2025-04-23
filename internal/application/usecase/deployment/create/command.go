package createdeployment

import (
	"github.com/glacius-labs/captain-compose/internal/application/command"
)

const CommandType command.Type = "create.deployment"

type Command struct {
	Name    string
	Payload []byte
}

func (c *Command) Type() command.Type {
	return CommandType
}
