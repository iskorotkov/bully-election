package states

import (
	"errors"
	"time"

	"github.com/iskorotkov/bully-election/pkg/services"
	"go.uber.org/zap"
)

var (
	ErrTransition = errors.New("invalid transition between states")
)

type State interface {
	Tick(elapsed time.Duration) (State, error)
	OnElection(source services.Instance) (State, error)
	OnAlive(source services.Instance) (State, error)
	OnVictory(source services.Instance) (State, error)
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

func (s *starting) OnElection(source services.Instance) (State, error) {
	return s, nil
}

func (s *starting) OnAlive(source services.Instance) (State, error) {
	return s, nil
}

func (s *starting) OnVictory(source services.Instance) (State, error) {
	return notElect(s.config), nil
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
		s.config.ServiceDiscovery.AnnounceLeadership()
		return elect(s.config), nil
	}

	s.config.ServiceDiscovery.StartElection()
	return onElectionStarted(s.config), nil
}

func (s *startingElection) OnElection(source services.Instance) (State, error) {
	return s, nil
}

func (s *startingElection) OnAlive(source services.Instance) (State, error) {
	return s, nil
}

func (s *startingElection) OnVictory(source services.Instance) (State, error) {
	return notElect(s.config), nil
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

func (s *startedElection) OnElection(source services.Instance) (State, error) {
	return s, nil
}

func (s *startedElection) OnAlive(source services.Instance) (State, error) {
	// TODO: Fill WaitingForElection.
	return waitForElection(s.config), nil
}

func (s *startedElection) OnVictory(source services.Instance) (State, error) {
	return notElect(s.config), nil
}

// Elected.

func elect(config Config) State {
	return &elected{
		config: config,
		logger: config.Logger.Named("elected"),
	}
}

type elected struct {
	config Config
	logger *zap.Logger
}

func (s *elected) Tick(elapsed time.Duration) (State, error) {
	return s, nil
}

func (s *elected) OnElection(source services.Instance) (State, error) {
	return startElection(s.config), nil
}

func (s *elected) OnAlive(source services.Instance) (State, error) {
	return s, nil
}

func (s *elected) OnVictory(source services.Instance) (State, error) {
	return notElect(s.config), ErrTransition
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

func (s *waitingForElection) OnElection(source services.Instance) (State, error) {
	return s, nil
}

func (s *waitingForElection) OnAlive(source services.Instance) (State, error) {
	return s, nil
}

func (s *waitingForElection) OnVictory(source services.Instance) (State, error) {
	return notElect(s.config), nil
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

func (s *notElected) OnElection(source services.Instance) (State, error) {
	return s, nil
}

func (s *notElected) OnAlive(source services.Instance) (State, error) {
	return s, nil
}

func (s *notElected) OnVictory(source services.Instance) (State, error) {
	return s, ErrTransition
}
