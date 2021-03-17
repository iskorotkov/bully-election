package services

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/iskorotkov/bully-election/pkg/messages"
	"github.com/iskorotkov/bully-election/pkg/network"
	"go.uber.org/zap"
)

var (
	ErrNoLeader = errors.New("couldn't execute operation on leader because it isn't set")
)

type Instance struct {
	Name string
}

type ServiceDiscovery struct {
	client      *network.Client
	leader      *Instance
	pingTimeout time.Duration
	logger      *zap.Logger
}

func NewServiceDiscovery(pingTimeout time.Duration, logger *zap.Logger) *ServiceDiscovery {
	return &ServiceDiscovery{
		client:      network.NewClient(logger.Named("client")),
		leader:      nil,
		pingTimeout: pingTimeout,
		logger:      logger,
	}
}

func (s *ServiceDiscovery) MakeLeader(leader *Instance) {
	s.leader = leader
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
		Destination: s.leader.Name,
		Content:     messages.MessagePing,
	}

	if err := s.client.Send(ctx, msg); err != nil {
		logger.Warn("couldn't send message", zap.Any("message", msg))
		return false, err
	}

	return true, nil
}

func (s *ServiceDiscovery) MustBeLeader() (bool, error) {
	// TODO: Must be leader?
	return true, nil
}

func (s *ServiceDiscovery) AnnounceLeadership() error {
	// TODO: Announce leadership.
	return nil
}

func (s *ServiceDiscovery) StartElection() error {
	// TODO: Start election.
	return nil
}

func (s *ServiceDiscovery) find() ([]Instance, error) {
	// TODO: Implement service discovery.
	return make([]Instance, 0), nil
}

func (s *ServiceDiscovery) this() (Instance, error) {
	host, err := os.Hostname()
	return Instance{
		Name: host,
	}, err
}

func (s *ServiceDiscovery) Close() error {
	return s.client.Close()
}
