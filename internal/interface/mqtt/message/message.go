package message

import "github.com/glacius-labs/captain-compose/internal/application/command"

type Type string

type Message interface {
	Type() Type
	ToCommand() command.Command
}
