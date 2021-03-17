package messages

type Message struct {
	Type string `json:"type"`
}

var (
	MessageElection = Message{"election"}
	MessageAlive    = Message{"alive"}
	MessageVictory  = Message{"victory"}
	MessagePing     = Message{"ping"}
	MessagePong     = Message{"pong"}
)
