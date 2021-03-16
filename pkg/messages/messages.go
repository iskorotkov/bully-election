package messages

type Message struct {
	Type string
}

func Election() Message {
	return Message{"election"}
}

func Alive() Message {
	return Message{"alive"}
}

func Victory() Message {
	return Message{"victory"}
}
