package removedeployment

import (
	"github.com/glacius-labs/captain-compose/internal/application/command"
	removedeployment "github.com/glacius-labs/captain-compose/internal/application/usecase/deployment/remove"
	"github.com/glacius-labs/captain-compose/internal/interface/mqtt/message"
)

const MessageType message.Type = "remove.deployment"

type Message struct {
	Name string `json:"name"`
}

func (m Message) ToCommand() command.Command {
	return &removedeployment.Command{
		Name: m.Name,
	}
}

func (m Message) Type() message.Type {
	return MessageType
}
