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

func FromPod(pod v1.Pod) (Replica, bool) {
	if pod.Status.PodIP == "" {
		return ReplicaNone, false
	}

	return NewReplica(pod.GetName(), pod.Status.PodIP), true
}

func FromPods(pods []v1.Pod) []Replica {
	var replicas []Replica
	for _, pod := range pods {
		replica, ok := FromPod(pod)
		if ok {
			replicas = append(replicas, replica)
		}
	}

	return replicas
}
