package message

import "github.com/glacius-labs/captain-compose/internal/application/command"

type Decoder interface {
	Decode(env *Envelope) (command.Command, error)
	CanDecode(t Type) bool
}
