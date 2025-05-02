package createdeployment

import (
	"encoding/json"

	"github.com/glacius-labs/captain-compose/internal/application/command"
	"github.com/glacius-labs/captain-compose/internal/interface/mqtt/message"
)

type Decoder struct {
}

func NewDecoder() *Decoder {
	return &Decoder{}
}

func (d *Decoder) CanDecode(t message.Type) bool {
	return t == MessageType
}

func (d *Decoder) Decode(env *message.Envelope) (command.Command, error) {
	var msg Message
	if err := json.Unmarshal(env.Payload, &msg); err != nil {
		return nil, err
	}

	return msg.ToCommand(), nil
}
