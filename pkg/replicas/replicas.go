package replicas

import v1 "k8s.io/api/core/v1"

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

func FromPod(pod v1.Pod) Replica {
	return NewReplica(pod.GetName(), pod.Status.PodIP)
}

func FromPods(pods []v1.Pod) []Replica {
	var replicas []Replica
	for _, pod := range pods {
		replicas = append(replicas, FromPod(pod))
	}

	return replicas
}
