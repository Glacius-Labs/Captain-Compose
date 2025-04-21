package removedeployment

import "github.com/glacius-labs/captain-compose/internal/application/command"

const CommandType command.Type = "remove.deployment"

type Command struct {
	Name string
}

func (c *Command) Type() command.Type {
	return CommandType
}
