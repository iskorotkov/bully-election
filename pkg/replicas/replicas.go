package replicas

var (
	ReplicaNone = Replica{}
)

type Replica struct {
	Name string
}

func NewReplica(name string) Replica {
	return Replica{
		Name: name,
	}
}
