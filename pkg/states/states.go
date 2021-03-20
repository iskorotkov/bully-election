package states

import (
	"errors"
	"sync"
	"time"

	"github.com/iskorotkov/bully-election/pkg/replicas"
	"github.com/iskorotkov/bully-election/pkg/services"
	"go.uber.org/zap"
)

var (
	ErrTransition = errors.New("invalid transition between states")
)

type State interface {
	Tick(elapsed time.Duration) (State, error)
	OnElection(source replicas.Replica) State
	OnAlive(source replicas.Replica) State
	OnVictory(source replicas.Replica) State
}

type Config struct {
	ElectionTimeout  time.Duration
	VictoryTimeout   time.Duration
	ServiceDiscovery *services.ServiceDiscovery
	Logger           *zap.Logger
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
		state:  start(config),
		mu:     &sync.RWMutex{},
		logger: config.Logger,
	}
}

func (f *FSM) Tick(elapsed time.Duration) error {
	f.logger.Debug("tick called",
		zap.Duration("elapsed", elapsed))

	state, err := f.state.Tick(elapsed)
	if err != nil {
		f.logger.Error("error occurred during FSM tick",
			zap.Error(err))
	}

	f.mu.Lock()
	defer f.mu.Unlock()

	f.state = state
	return err
}

func (f *FSM) OnElection(source replicas.Replica) {
	f.logger.Debug("on election called",
		zap.Any("source", source))

	f.mu.Lock()
	defer f.mu.Unlock()

	f.state = f.state.OnElection(source)
}

func (f *FSM) OnAlive(source replicas.Replica) {
	f.logger.Debug("on alive called",
		zap.Any("source", source))

	f.mu.Lock()
	defer f.mu.Unlock()

	f.state = f.state.OnAlive(source)
}

func (f *FSM) OnVictory(source replicas.Replica) {
	f.logger.Debug("on victory called",
		zap.Any("source", source))

	f.mu.Lock()
	defer f.mu.Unlock()

	f.state = f.state.OnVictory(source)
}

func (f *FSM) State() State {
	f.mu.RLock()
	defer f.mu.RUnlock()

	return f.state
}

// Starting.

func start(config Config) State {
	logger := config.Logger.Named("starting")
	logger.Info("enter starting state")
	return &starting{
		config: config,
		logger: logger,
	}
}

type starting struct {
	config Config
	logger *zap.Logger
}

func (s *starting) Tick(elapsed time.Duration) (State, error) {
	return startElection(s.config), nil
}

func (s *starting) OnElection(source replicas.Replica) State {
	return s
}

func (s *starting) OnAlive(source replicas.Replica) State {
	return s
}

func (s *starting) OnVictory(source replicas.Replica) State {
	return notElect(s.config)
}

// Starting election.

func startElection(config Config) State {
	logger := config.Logger.Named("starting-election")
	logger.Info("enter start election state")
	return &startingElection{
		config: config,
		logger: logger,
	}
}

type startingElection struct {
	config Config
	logger *zap.Logger
}

func (s *startingElection) Tick(elapsed time.Duration) (State, error) {
	ok, err := s.config.ServiceDiscovery.MustBeLeader()
	if err != nil {
		return s, err
	}

	if ok {
		return elect(s.config), nil
	}

	s.config.ServiceDiscovery.StartElection()
	return onElectionStarted(s.config), nil
}

func (s *startingElection) OnElection(source replicas.Replica) State {
	return s
}

func (s *startingElection) OnAlive(source replicas.Replica) State {
	return s
}

func (s *startingElection) OnVictory(source replicas.Replica) State {
	return notElect(s.config)
}

// Started election.

func onElectionStarted(config Config) State {
	logger := config.Logger.Named("started-election")
	logger.Info("enter election started state")
	return &startedElection{
		elapsed: config.ElectionTimeout,
		config:  config,
		logger:  logger,
	}
}

type startedElection struct {
	elapsed time.Duration
	config  Config
	logger  *zap.Logger
}

func (s *startedElection) Tick(elapsed time.Duration) (State, error) {
	s.elapsed -= elapsed
	if s.elapsed <= 0 {
		return elect(s.config), nil
	}

	return s, nil
}

func (s *startedElection) OnElection(source replicas.Replica) State {
	return s
}

func (s *startedElection) OnAlive(source replicas.Replica) State {
	return waitForElection(s.config)
}

func (s *startedElection) OnVictory(source replicas.Replica) State {
	return notElect(s.config)
}

// Elected.

func elect(config Config) State {
	logger := config.Logger.Named("elected")
	logger.Info("enter elected state")
	return &elected{
		announced: false,
		config:    config,
		logger:    logger,
	}
}

type elected struct {
	announced bool
	config    Config
	logger    *zap.Logger
}

func (s *elected) Tick(elapsed time.Duration) (State, error) {
	if !s.announced {
		if err := s.config.ServiceDiscovery.AnnounceLeadership(); err != nil {
			s.logger.Error("couldn't announce leadership",
				zap.Error(err))
			return s, err
		}

		s.announced = true
	}

	return s, nil
}

func (s *elected) OnElection(source replicas.Replica) State {
	return startElection(s.config)
}

func (s *elected) OnAlive(source replicas.Replica) State {
	return s
}

func (s *elected) OnVictory(source replicas.Replica) State {
	return notElect(s.config)
}

// Waiting for election.

func waitForElection(config Config) State {
	logger := config.Logger.Named("waiting-for-election")
	logger.Info("enter waiting for election state")
	return &waitingForElection{
		elapsed: config.VictoryTimeout,
		config:  config,
		logger:  logger,
	}
}

type waitingForElection struct {
	elapsed time.Duration
	config  Config
	logger  *zap.Logger
}

func (s *waitingForElection) Tick(elapsed time.Duration) (State, error) {
	s.elapsed -= elapsed
	if s.elapsed <= 0 {
		return startElection(s.config), nil
	}

	return s, nil
}

func (s *waitingForElection) OnElection(source replicas.Replica) State {
	return s
}

func (s *waitingForElection) OnAlive(source replicas.Replica) State {
	return s
}

func (s *waitingForElection) OnVictory(source replicas.Replica) State {
	return notElect(s.config)
}

// Not elected.

func notElect(config Config) State {
	logger := config.Logger.Named("not-elected")
	logger.Info("enter not elected state")
	return &notElected{
		config: config,
		logger: logger,
	}
}

type notElected struct {
	config Config
	logger *zap.Logger
}

func (s *notElected) Tick(elapsed time.Duration) (State, error) {
	ok, err := s.config.ServiceDiscovery.PingLeader()
	if err != nil {
		return s, err
	}

	if !ok {
		return startElection(s.config), nil
	}

	return s, nil
}

func (s *notElected) OnElection(source replicas.Replica) State {
	return s
}

func (s *notElected) OnAlive(source replicas.Replica) State {
	return s
}

func (s *notElected) OnVictory(source replicas.Replica) State {
	return s
}
