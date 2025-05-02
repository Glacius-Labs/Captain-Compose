package createdeployment

import (
	"github.com/glacius-labs/captain-compose/internal/application/command"
	createdeployment "github.com/glacius-labs/captain-compose/internal/application/usecase/deployment/create"
	"github.com/glacius-labs/captain-compose/internal/interface/mqtt/message"
)

const MessageType message.Type = "create.deployment"

type Message struct {
	Name    string `json:"name"`
	Payload string `json:"payload"`
}

func (m Message) Type() message.Type {
	return MessageType
}

func (m Message) ToCommand() command.Command {
	return &createdeployment.Command{
		Name:    m.Name,
		Payload: []byte(m.Payload),
	}
}
