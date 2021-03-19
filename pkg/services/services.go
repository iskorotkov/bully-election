package services

import (
	"context"
	"errors"
	"os"
	"sync"
	"time"

	"github.com/iskorotkov/bully-election/pkg/messages"
	"github.com/iskorotkov/bully-election/pkg/network"
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
	labelKey    string
	client      *network.Client
	k8s         *kubernetes.Clientset
	leader      *replicas.Replica
	pingTimeout time.Duration
	logger      *zap.Logger
}

func NewServiceDiscovery(labelKey string, pingTimeout time.Duration, client *network.Client, logger *zap.Logger) (*ServiceDiscovery, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		logger.Warn("couldn't create kubernetes config",
			zap.Error(err))
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		logger.Warn("couldn't create kubernetes clientset",
			zap.Error(err))
		return nil, err
	}

	return &ServiceDiscovery{
		labelKey:    labelKey,
		client:      client,
		k8s:         clientset,
		leader:      nil,
		pingTimeout: pingTimeout,
		logger:      logger,
	}, nil
}

func (s *ServiceDiscovery) Leader() *replicas.Replica {
	return s.leader
}

func (s *ServiceDiscovery) PingLeader() (bool, error) {
	logger := s.logger.Named("ping-leader")

	if s.leader == nil {
		logger.Warn("leader is nil")
		return false, ErrNoLeader
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
	defer cancel()

	msg := network.OutgoingMessage{
		Destination: *s.leader,
		Content:     messages.MessagePing,
	}

	if err := s.client.Send(ctx, msg); err != nil {
		logger.Warn("couldn't send message", zap.Any("message", msg))
		return false, err
	}

	return true, nil
}

func (s *ServiceDiscovery) MustBeLeader() (bool, error) {
	potentialLeaders, err := s.potentialLeaders()
	if err != nil {
		s.logger.Warn("couldn't find potential leaders",
			zap.Error(err))
		return false, err
	}

	return len(potentialLeaders) == 0, nil
}

func (s *ServiceDiscovery) AnnounceLeadership() error {
	logger := s.logger.Named("start-election")

	self, err := s.Self()
	if err != nil {
		logger.Warn("couldn't determine own identity",
			zap.Error(err))
		return err
	}

	s.leader = &self

	all, err := s.others()
	if err != nil {
		logger.Warn("couldn't fetch other replicas",
			zap.Error(err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(len(all))

	for _, leader := range all {
		msg := network.OutgoingMessage{
			Destination: leader,
			Content:     messages.MessageVictory,
		}

		go func() {
			defer wg.Done()

			if err := s.client.Send(ctx, msg); err != nil {
				logger.Warn("couldn't send victory message",
					zap.Any("message", msg),
					zap.Error(err))
			}
		}()
	}

	wg.Wait()

	return nil
}

func (s *ServiceDiscovery) StartElection() error {
	logger := s.logger.Named("start-election")

	potentialLeaders, err := s.potentialLeaders()
	if err != nil {
		logger.Warn("couldn't fetch potential leaders",
			zap.Error(err))
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
	defer cancel()

	wg := sync.WaitGroup{}
	wg.Add(len(potentialLeaders))

	for _, leader := range potentialLeaders {
		msg := network.OutgoingMessage{
			Destination: leader,
			Content:     messages.MessageElection,
		}

		go func() {
			if err := s.client.Send(ctx, msg); err != nil {
				logger.Warn("couldn't send election message",
					zap.Any("message", msg),
					zap.Error(err))
			}
		}()
	}

	return nil
}

func (s *ServiceDiscovery) Self() (replicas.Replica, error) {
	logger := s.logger.Named("self")

	hostname, err := os.Hostname()
	if err != nil {
		logger.Warn("couldn't get hostname",
			zap.Error(err))
		return replicas.Replica{}, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
	defer cancel()

	pod, err := s.k8s.CoreV1().Pods("").Get(ctx, hostname, v1.GetOptions{})
	if err != nil {
		logger.Warn("couldn't get pod info",
			zap.String("pod", hostname),
			zap.Error(err))
		return replicas.Replica{}, err
	}

	label, ok := pod.GetLabels()[s.labelKey]
	if !ok {
		logger.Warn("couldn't find required label",
			zap.String("key", s.labelKey),
			zap.Any("pod", pod))
		return replicas.Replica{}, err
	}

	return replicas.NewReplica(label), err
}

func (s *ServiceDiscovery) others() ([]replicas.Replica, error) {
	logger := s.logger.Named("others")

	ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
	defer cancel()

	endpoints, err := s.k8s.CoreV1().Endpoints("").List(ctx, v1.ListOptions{})
	if err != nil {
		logger.Warn("couldn't get list of pods",
			zap.Error(err))
		return nil, err
	}

	results := make([]replicas.Replica, 0)
	for _, endpoint := range endpoints.Items {
		if len(endpoint.Subsets[0].Addresses) == 0 {
			continue
		}

		replica := replicas.NewReplica(endpoint.GetName())
		results = append(results, replica)
	}

	return results, nil
}

func (s *ServiceDiscovery) potentialLeaders() ([]replicas.Replica, error) {
	logger := s.logger.Named("potential-leaders")

	self, err := s.Self()
	if err != nil {
		logger.Warn("couldn't determine own identity",
			zap.Error(err))
		return nil, err
	}

	others, err := s.others()
	if err != nil {
		logger.Warn("couldn't get others",
			zap.Error(err))
		return nil, err
	}

	potentialLeaders := make([]replicas.Replica, 0)
	for _, other := range others {
		if self.Name < other.Name {
			potentialLeaders = append(potentialLeaders, other)
		}
	}

	return potentialLeaders, nil
}
