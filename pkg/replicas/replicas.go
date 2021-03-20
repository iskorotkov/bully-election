package replicas

var (
	ReplicaNone = Replica{}
)

type Replica struct {
	Name string `json:"name"`
	IP   string `json:"ip"`
}

func NewReplica(name string, ip string) Replica {
	return Replica{
		Name: name,
		IP:   ip,
	}
}
