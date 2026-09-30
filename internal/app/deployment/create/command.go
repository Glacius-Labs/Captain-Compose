package create

type Command struct {
	Name    string `json:"name"`
	Payload []byte `json:"payload"`
}
