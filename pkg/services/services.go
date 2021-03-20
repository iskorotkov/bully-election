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
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

var (
	ErrNoLeader = errors.New("couldn't execute operation on leader because it isn't set")
)

type ServiceDiscovery struct {
	namespace   string
	client      *comms.Client
	k8s         *kubernetes.Clientset
	self        replicas.Replica
	leader      replicas.Replica
	pingTimeout time.Duration
	logger      *zap.Logger
}

func NewServiceDiscovery(ns string, timeout time.Duration,
	client *comms.Client, logger *zap.Logger) (*ServiceDiscovery, error) {
	hostname, err := os.Hostname()
	if err != nil {
		logger.Error("couldn't get hostname",
			zap.Error(err))
		return nil, err
	}

	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Error("couldn't create kubernetes config",
			zap.Error(err))
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Error("couldn't create kubernetes clientset",
			zap.Error(err))
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	podInfo, err := clientset.CoreV1().Pods(ns).Get(ctx, hostname, v1.GetOptions{})
	if err != nil {
		logger.Error("couldn't get pod info",
			zap.String("hostname", hostname),
			zap.Error(err))
		return nil, err
	}

	self := replicas.NewReplica(podInfo.GetName(), podInfo.Status.PodIP)

	return &ServiceDiscovery{
		namespace:   ns,
		client:      client,
		k8s:         clientset,
		leader:      replicas.ReplicaNone,
		self:        self,
		pingTimeout: timeout,
		logger:      logger,
	}, nil
}

func (s *ServiceDiscovery) Leader() replicas.Replica {
	return s.leader
}

func (s *ServiceDiscovery) PingLeader() (bool, error) {
	logger := s.logger.Named("ping-leader")
	logger.Info("leader ping initiated")

	if s.leader == replicas.ReplicaNone {
		logger.Error("leader is nil")
		return false, ErrNoLeader
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
	defer cancel()

	msg := comms.NewOutgoingMessage(s.self, s.leader, messages.MessagePing)

	if err := s.client.Send(ctx, msg); err != nil {
		logger.Error("couldn't send message", zap.Any("message", msg))
		return false, err
	}

	return true, nil
}

func (s *ServiceDiscovery) MustBeLeader() (bool, error) {
	s.logger.Info("check if must become a leader")

	potentialLeaders, err := s.potentialLeaders()
	if err != nil {
		s.logger.Error("couldn't find potential leaders",
			zap.Error(err))
		return false, err
	}

	return len(potentialLeaders) == 0, nil
}

func (s *ServiceDiscovery) AnnounceLeadership() error {
	logger := s.logger.Named("start-election")
	logger.Info("announce leadership")

	s.leader = s.Self()

	others, err := s.others()
	if err != nil {
		logger.Error("couldn't fetch other replicas",
			zap.Error(err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(len(others))

	for _, pod := range others {
		msg := comms.NewOutgoingMessage(s.self, pod, messages.MessageVictory)

		go func() {
			defer wg.Done()

			if err := s.client.Send(ctx, msg); err != nil {
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

	potentialLeaders, err := s.potentialLeaders()
	if err != nil {
		logger.Error("couldn't fetch potential leaders",
			zap.Error(err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(len(potentialLeaders))

	for _, pl := range potentialLeaders {
		msg := comms.NewOutgoingMessage(s.self, pl, messages.MessageElection)

		go func() {
			defer wg.Done()

			if err := s.client.Send(ctx, msg); err != nil {
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

func (s *ServiceDiscovery) others() ([]replicas.Replica, error) {
	logger := s.logger.Named("others")
	logger.Debug("looking for others")

	ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
	defer cancel()

	pods, err := s.k8s.CoreV1().Pods(s.namespace).List(ctx, v1.ListOptions{})
	if err != nil {
		logger.Error("couldn't get list of pods",
			zap.Error(err))
		return nil, err
	}

	results := make([]replicas.Replica, 0)
	for _, pod := range pods.Items {
		if pod.GetName() == s.Self().Name {
			continue
		}

		replica := replicas.NewReplica(pod.GetName(), pod.Status.PodIP)
		results = append(results, replica)
	}

	logger.Debug("others found",
		zap.Any("others", results))

	return results, nil
}

func (s *ServiceDiscovery) potentialLeaders() ([]replicas.Replica, error) {
	logger := s.logger.Named("potential-leaders")
	logger.Debug("looking for potential leaders")

	others, err := s.others()
	if err != nil {
		logger.Error("couldn't get others",
			zap.Error(err))
		return nil, err
	}

	potentialLeaders := make([]replicas.Replica, 0)
	for _, other := range others {
		if s.Self().Name < other.Name {
			potentialLeaders = append(potentialLeaders, other)
		}
	}

	logger.Debug("potential leaders found",
		zap.Any("leaders", potentialLeaders))

	return potentialLeaders, nil
}
