package states

import (
	"errors"
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

// Starting.

func Start(config Config) State {
	return &starting{
		config: config,
		logger: config.Logger.Named("starting"),
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
	return &startingElection{
		config: config,
		logger: config.Logger.Named("starting-election"),
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
	return &startedElection{
		elapsed: config.ElectionTimeout,
		config:  config,
		logger:  config.Logger.Named("started-election"),
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
	return &elected{
		announced: false,
		config:    config,
		logger:    config.Logger.Named("elected"),
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
			s.logger.Warn("couldn't announce leadership",
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
	return &waitingForElection{
		elapsed: config.VictoryTimeout,
		config:  config,
		logger:  config.Logger.Named("waiting-for-election"),
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
	return &notElected{
		config: config,
		logger: config.Logger.Named("not-elected"),
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
