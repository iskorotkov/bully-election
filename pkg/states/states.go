package states

import (
	"sync"
	"time"

	"github.com/iskorotkov/bully-election/pkg/replicas"
	"github.com/iskorotkov/bully-election/pkg/services"
	"go.uber.org/zap"
)

type Role string

var (
	RoleLeader        = Role("leader")
	RoleReplica       = Role("replica")
	RoleTransitioning = Role("transitioning")
)

type State interface {
	Tick(elapsed time.Duration) State
	OnElectionMessage(source replicas.Replica) State
	OnVictoryMessage(source replicas.Replica) State
	OnAliveResponse(source replicas.Replica) State
	OnElectionResponse(source replicas.Replica) State
	Role() Role
}

type Config struct {
	WaitBeforeAutoElection time.Duration
	WaitForOtherElection   time.Duration
	WaitForLeaderResponse  time.Duration
	WaitBeforeNextPing     time.Duration
	ServiceDiscovery       *services.ServiceDiscovery
	Logger                 *zap.Logger
}

type FSM struct {
	state  State
	mu     *sync.RWMutex
	logger *zap.Logger
}

func NewFSM(config Config) *FSM {
	config.Logger.Info("start new fsm",
		zap.Any("config", config))

	return &FSM{
		state:  startElection(config),
		mu:     &sync.RWMutex{},
		logger: config.Logger,
	}
}

func (f *FSM) Tick(elapsed time.Duration) {
	f.logger.Debug("tick called",
		zap.Duration("elapsed", elapsed))

	f.mu.Lock()
	defer f.mu.Unlock()

	f.state = f.state.Tick(elapsed)
}

func (f *FSM) OnElectionMessage(source replicas.Replica) {
	f.logger.Debug("on election message",
		zap.Any("source", source))

	f.mu.Lock()
	defer f.mu.Unlock()

	f.state = f.state.OnElectionMessage(source)
}

func (f *FSM) OnVictoryMessage(source replicas.Replica) {
	f.logger.Debug("on victory message",
		zap.Any("source", source))

	f.mu.Lock()
	defer f.mu.Unlock()

	f.state = f.state.OnVictoryMessage(source)
}

func (f *FSM) OnAliveResponse(source replicas.Replica) {
	f.logger.Debug("on alive response",
		zap.Any("source", source))

	f.mu.Lock()
	defer f.mu.Unlock()

	f.state = f.state.OnAliveResponse(source)
}

func (f *FSM) OnElectionResponse(source replicas.Replica) {
	f.logger.Debug("on election response",
		zap.Any("source", source))

	f.mu.Lock()
	defer f.mu.Unlock()

	f.state = f.state.OnElectionResponse(source)
}

func (f *FSM) State() State {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.state
}

// Starting election.

func startElection(config Config) State {
	config.Logger.Debug("start election")

	if config.ServiceDiscovery.MustBeLeader() {
		return elect(config)
	}

	config.ServiceDiscovery.StartElection()

	return waitForResponse(config)
}

// Started election.

func waitForResponse(config Config) State {
	logger := config.Logger.Named("waiting-for-election")
	logger.Info("enter waiting for response state")

	return &waitingForResponse{
		elapsed: config.WaitBeforeAutoElection,
		config:  config,
		logger:  logger,
	}
}

type waitingForResponse struct {
	elapsed time.Duration
	config  Config
	logger  *zap.Logger
}

func (s *waitingForResponse) Tick(elapsed time.Duration) State {
	s.elapsed -= elapsed
	if s.elapsed <= 0 {
		return elect(s.config)
	}

	return s
}

func (s *waitingForResponse) OnElectionMessage(source replicas.Replica) State {
	return s
}

func (s *waitingForResponse) OnVictoryMessage(source replicas.Replica) State {
	s.config.ServiceDiscovery.RememberLeader(source)
	return waitToPing(s.config)
}

func (s *waitingForResponse) OnAliveResponse(source replicas.Replica) State {
	s.logger.Warn("unexpected alive response",
		zap.Any("source", source))

	return s
}

func (s *waitingForResponse) OnElectionResponse(source replicas.Replica) State {
	return waitForElection(s.config)
}

func (s *waitingForResponse) Role() Role {
	return RoleTransitioning
}

// Elected.

func elect(config Config) State {
	logger := config.Logger.Named("elected")
	logger.Info("enter elected state")

	config.ServiceDiscovery.AnnounceLeadership()

	return &elected{
		config: config,
		logger: logger,
	}
}

type elected struct {
	config Config
	logger *zap.Logger
}

func (s *elected) Tick(elapsed time.Duration) State {
	return s
}

func (s *elected) OnElectionMessage(source replicas.Replica) State {
	return startElection(s.config)
}

func (s *elected) OnVictoryMessage(source replicas.Replica) State {
	s.logger.Warn("elected replica received victory message",
		zap.Any("source", source))

	return startElection(s.config)
}

func (s *elected) OnAliveResponse(source replicas.Replica) State {
	s.logger.Warn("unexpected alive response",
		zap.Any("source", source))

	return s
}

func (s *elected) OnElectionResponse(source replicas.Replica) State {
	s.logger.Warn("unexpected election response",
		zap.Any("source", source))

	return s
}

func (s *elected) Role() Role {
	return RoleLeader
}

// Waiting for election.

func waitForElection(config Config) State {
	logger := config.Logger.Named("waiting-for-election")
	logger.Info("enter waiting for election state")

	return &waitingForElection{
		elapsed: config.WaitForOtherElection,
		config:  config,
		logger:  logger,
	}
}

type waitingForElection struct {
	elapsed time.Duration
	config  Config
	logger  *zap.Logger
}

func (s *waitingForElection) Tick(elapsed time.Duration) State {
	s.elapsed -= elapsed
	if s.elapsed <= 0 {
		return startElection(s.config)
	}

	return s
}

func (s *waitingForElection) OnElectionMessage(source replicas.Replica) State {
	return s
}

func (s *waitingForElection) OnVictoryMessage(source replicas.Replica) State {
	s.config.ServiceDiscovery.RememberLeader(source)
	return waitToPing(s.config)
}

func (s *waitingForElection) OnAliveResponse(source replicas.Replica) State {
	s.logger.Warn("unexpected alive response",
		zap.Any("source", source))

	return s
}

func (s *waitingForElection) OnElectionResponse(source replicas.Replica) State {
	s.logger.Warn("unexpected election response",
		zap.Any("source", source))

	return s
}

func (s *waitingForElection) Role() Role {
	return RoleReplica
}

// Not elected.

func waitToPing(config Config) State {
	logger := config.Logger.Named("waiting-to-ping")
	logger.Info("enter waiting to ping state")

	return &waitingToPing{
		interval: config.WaitBeforeNextPing,
		config:   config,
		logger:   logger,
	}
}

type waitingToPing struct {
	interval time.Duration
	config   Config
	logger   *zap.Logger
}

func (s *waitingToPing) Tick(elapsed time.Duration) State {
	s.interval -= elapsed
	if s.interval <= 0 {
		if err := s.config.ServiceDiscovery.PingLeader(); err != nil {
			return startElection(s.config)
		}

		return waitForLeader(s.config)
	}

	return s
}

func (s *waitingToPing) OnElectionMessage(source replicas.Replica) State {
	return startElection(s.config)
}

func (s *waitingToPing) OnVictoryMessage(source replicas.Replica) State {
	s.logger.Warn("unexpected victory message",
		zap.Any("source", source))

	s.config.ServiceDiscovery.RememberLeader(source)
	return waitToPing(s.config)
}

func (s *waitingToPing) OnAliveResponse(source replicas.Replica) State {
	s.logger.Warn("unexpected alive response",
		zap.Any("source", source))

	return s
}

func (s *waitingToPing) OnElectionResponse(source replicas.Replica) State {
	s.logger.Warn("unexpected election response",
		zap.Any("source", source))

	return s
}

func (s *waitingToPing) Role() Role {
	return RoleReplica
}

// Not elected.

func waitForLeader(config Config) State {
	logger := config.Logger.Named("waiting-for-leader")
	logger.Info("enter not elected state")

	return &waitingForLeader{
		timeout: config.WaitForLeaderResponse,
		config:  config,
		logger:  logger,
	}
}

type waitingForLeader struct {
	timeout time.Duration
	config  Config
	logger  *zap.Logger
}

func (s *waitingForLeader) Tick(elapsed time.Duration) State {
	s.timeout -= elapsed
	if s.timeout <= 0 {
		return startElection(s.config)
	}

	return s
}

func (s *waitingForLeader) OnElectionMessage(source replicas.Replica) State {
	return startElection(s.config)
}

func (s *waitingForLeader) OnVictoryMessage(source replicas.Replica) State {
	s.logger.Warn("unexpected victory message",
		zap.Any("source", source))

	s.config.ServiceDiscovery.RememberLeader(source)
	return waitToPing(s.config)
}

func (s *waitingForLeader) OnAliveResponse(source replicas.Replica) State {
	return waitToPing(s.config)
}

func (s *waitingForLeader) OnElectionResponse(source replicas.Replica) State {
	s.logger.Warn("unexpected election response",
		zap.Any("source", source))

	return s
}

func (s *waitingForLeader) Role() Role {
	return RoleReplica
}
