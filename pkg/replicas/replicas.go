package replicas

type Replica struct {
	Name string
}

func NewReplica(name string) Replica {
	return Replica{
		Name: name,
	}
}
