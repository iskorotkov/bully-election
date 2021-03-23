package services

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/iskorotkov/bully-election/pkg/comms"
	"github.com/iskorotkov/bully-election/pkg/messages"
	"github.com/iskorotkov/bully-election/pkg/replicas"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	ErrNoLeader = errors.New("couldn't execute operation on leader because it isn't set")
)

type ServiceDiscovery struct {
	namespace string

	// Request timeouts.
	pingTimeout       time.Duration
	electionTimeout   time.Duration
	leadershipTimeout time.Duration
	refreshTimeout    time.Duration

	// Clients.
	client     *comms.Client
	kubeClient *kubernetes.Clientset

	// Self.
	self replicas.Replica

	// Leader.
	leader replicas.Replica

	// Others.
	othersInternal []replicas.Replica
	othersM        *sync.RWMutex

	// Logging.
	logger *zap.Logger
}

type Config struct {
	Namespace string

	// Request timeouts.
	PingTimeout       time.Duration
	ElectionTimeout   time.Duration
	LeadershipTimeout time.Duration
	RefreshTimeout    time.Duration
	SelfInfoTimeout   time.Duration

	// Intervals.
	RefreshInterval  time.Duration
	SelfInfoInverval time.Duration

	Client *comms.Client
	Logger *zap.Logger
}

func NewServiceDiscovery(config Config) (*ServiceDiscovery, error) {
	// Hostname.
	hostname, err := os.Hostname()
	if err != nil {
		config.Logger.Error("couldn't get hostname",
			zap.Error(err))
		return nil, err
	}

	// Kubernetes client.
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		config.Logger.Error("couldn't create kubernetes config",
			zap.Error(err))
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		config.Logger.Error("couldn't create kubernetes clientset",
			zap.Error(err))
		return nil, err
	}

	var (
		thisPod   corev1.Pod
		otherPods []corev1.Pod
	)

	// Fetch info about pods.
	// Continue until current pod gets assigned IP address.
	for thisPod.Status.PodIP == "" {
		ctx, cancel := context.WithTimeout(context.Background(), config.SelfInfoTimeout)
		defer cancel()

		pods, err := clientset.CoreV1().Pods(config.Namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			config.Logger.Error("couldn't get pod info",
				zap.String("hostname", hostname),
				zap.Error(err))
			return nil, err
		}

		for index, pod := range pods.Items {
			if pod.GetName() == hostname {
				thisPod = pod
				otherPods = append(pods.Items[:index], pods.Items[index+1:]...)
			}
		}

		time.Sleep(config.SelfInfoInverval)
	}

	self, _ := replicas.FromPod(thisPod)

	config.Logger.Info("fetched info about self",
		zap.Any("self", self))

	s := &ServiceDiscovery{
		namespace: config.Namespace,
		// Timeouts.
		pingTimeout:       config.PingTimeout,
		electionTimeout:   config.ElectionTimeout,
		leadershipTimeout: config.LeadershipTimeout,
		refreshTimeout:    config.RefreshTimeout,
		// Clients.
		client:     config.Client,
		kubeClient: clientset,
		// Replicas.
		self:   self,
		leader: replicas.ReplicaNone,
		// Others.
		othersInternal: replicas.FromPods(otherPods),
		othersM:        &sync.RWMutex{},
		// Logging.
		logger: config.Logger,
	}

	go func() {
		for {
			others, err := s.refreshOthers()
			if err != nil {
				config.Logger.Error("",
					zap.Error(err))
			} else {
				func() {
					s.othersM.Lock()
					defer s.othersM.Unlock()
					s.othersInternal = others
				}()
			}

			time.Sleep(config.RefreshInterval)
		}
	}()

	return s, nil
}

func (s *ServiceDiscovery) RememberLeader(leader replicas.Replica) {
	s.leader = leader
}

func (s *ServiceDiscovery) PingLeader() (bool, error) {
	logger := s.logger.Named("ping-leader")
	logger.Info("leader ping initiated")

	if s.leader == replicas.ReplicaNone {
		logger.Error("leader is nil")
		return false, ErrNoLeader
	}

	msg := comms.NewOutgoingMessage(s.self, s.leader, messages.MessagePing)

	ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
	defer cancel()

	go func() {
		if err := s.client.Send(ctx, msg, true); err != nil {
			logger.Error("couldn't send message", zap.Any("message", msg))
			return
		}
	}()

	return true, nil
}

func (s *ServiceDiscovery) MustBeLeader() (bool, error) {
	s.logger.Info("check if must become a leader")

	potentialLeaders := s.PotentialLeadersSnapshot()
	return len(potentialLeaders) == 0, nil
}

func (s *ServiceDiscovery) AnnounceLeadership() error {
	logger := s.logger.Named("announce-leadership")
	logger.Info("start announcement")

	s.RememberLeader(s.self)

	others := s.OthersSnapshot()

	wg := sync.WaitGroup{}
	wg.Add(len(others))

	ctx, cancel := context.WithTimeout(context.Background(), s.leadershipTimeout)
	defer cancel()

	for _, pod := range others {
		msg := comms.NewOutgoingMessage(s.self, pod, messages.MessageVictory)

		go func() {
			defer wg.Done()

			if err := s.client.Send(ctx, msg, false); err != nil {
				logger.Error("couldn't send victory message",
					zap.Any("message", msg),
					zap.Error(err))
			}
		}()
	}

	wg.Wait()
	logger.Info("leadership announced")

	return nil
}

func (s *ServiceDiscovery) StartElection() error {
	logger := s.logger.Named("start-election")
	logger.Info("election started")

	potentialLeaders := s.PotentialLeadersSnapshot()

	wg := sync.WaitGroup{}
	wg.Add(len(potentialLeaders))

	ctx, cancel := context.WithTimeout(context.Background(), s.electionTimeout)
	defer cancel()

	for _, pl := range potentialLeaders {
		msg := comms.NewOutgoingMessage(s.self, pl, messages.MessageElection)

		go func() {
			defer wg.Done()

			if err := s.client.Send(ctx, msg, false); err != nil {
				logger.Error("couldn't send election message",
					zap.Any("message", msg),
					zap.Error(err))
			}
		}()
	}

	wg.Wait()
	logger.Info("election finished")

	return nil
}

func (s *ServiceDiscovery) Self() replicas.Replica {
	return s.self
}

func (s *ServiceDiscovery) Leader() replicas.Replica {
	return s.leader
}

func (s *ServiceDiscovery) OthersSnapshot() []replicas.Replica {
	s.othersM.RLock()
	defer s.othersM.RUnlock()

	return s.othersInternal
}

func (s *ServiceDiscovery) PotentialLeadersSnapshot() []replicas.Replica {
	logger := s.logger.Named("potential-leaders")
	logger.Debug("looking for potential leaders")

	others := s.OthersSnapshot()

	potentialLeaders := make([]replicas.Replica, 0)
	for _, other := range others {
		if s.self.Name < other.Name {
			potentialLeaders = append(potentialLeaders, other)
		}
	}

	logger.Debug("potential leaders found",
		zap.Any("leaders", potentialLeaders))

	return potentialLeaders
}

func (s *ServiceDiscovery) refreshOthers() ([]replicas.Replica, error) {
	logger := s.logger.Named("refresh-others")
	logger.Debug("looking for others")

	ctx, cancel := context.WithTimeout(context.Background(), s.refreshTimeout)
	defer cancel()

	pods, err := s.kubeClient.CoreV1().Pods(s.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		logger.Error("couldn't get list of pods",
			zap.Error(err))
		return nil, err
	}

	var otherPods []corev1.Pod
	for index, pod := range pods.Items {
		if pod.GetName() == s.self.Name {
			otherPods = append(pods.Items[:index], pods.Items[index+1:]...)
		}
	}

	results := replicas.FromPods(otherPods)

	logger.Debug("others found",
		zap.Any("others", results))

	return results, nil
}
