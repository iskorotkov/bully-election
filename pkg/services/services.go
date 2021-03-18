package services

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/iskorotkov/bully-election/pkg/messages"
	"github.com/iskorotkov/bully-election/pkg/network"
	"github.com/iskorotkov/bully-election/pkg/replicas"
	"go.uber.org/zap"
)

var (
	ErrNoLeader = errors.New("couldn't execute operation on leader because it isn't set")
)

type ServiceDiscovery struct {
	client      *network.Client
	leader      *replicas.Replica
	pingTimeout time.Duration
	logger      *zap.Logger
}

func NewServiceDiscovery(pingTimeout time.Duration, client *network.Client, logger *zap.Logger) *ServiceDiscovery {
	return &ServiceDiscovery{
		client:      client,
		leader:      nil,
		pingTimeout: pingTimeout,
		logger:      logger,
	}
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

	msg := network.OutcomingMessage{
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

	self, err := s.self()
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

	for _, leader := range all {
		ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
		defer cancel()

		msg := network.OutcomingMessage{
			Destination: leader,
			Content:     messages.MessageVictory,
		}

		if err := s.client.Send(ctx, msg); err != nil {
			logger.Warn("couldn't send victory message",
				zap.Any("target", leader),
				zap.Error(err))
			return err
		}
	}

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

	for _, leader := range potentialLeaders {
		ctx, cancel := context.WithTimeout(context.Background(), s.pingTimeout)
		defer cancel()

		msg := network.OutcomingMessage{
			Destination: leader,
			Content:     messages.MessageElection,
		}

		if err := s.client.Send(ctx, msg); err != nil {
			logger.Warn("couldn't send election message",
				zap.Any("target", leader),
				zap.Error(err))
			return err
		}
	}

	return nil
}

func (s *ServiceDiscovery) self() (replicas.Replica, error) {
	host, err := os.Hostname()
	return replicas.NewReplica(host), err
}

func (s *ServiceDiscovery) others() ([]replicas.Replica, error) {
	// TODO: Implement service discovery.
	return make([]replicas.Replica, 0), nil
}

func (s *ServiceDiscovery) potentialLeaders() ([]replicas.Replica, error) {
	logger := s.logger.Named("potential-leaders")

	self, err := s.self()
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
