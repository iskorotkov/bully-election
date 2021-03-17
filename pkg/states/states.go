package states

import (
	"errors"
	"time"

	"github.com/iskorotkov/bully-election/pkg/services"
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

// Starting.

type Starting struct{}

func (s *Starting) Tick(elapsed time.Duration) (State, error) {
	return &StartingElection{}, nil
}

func (s *Starting) OnElection(source services.Instance) (State, error) {
	return s, nil
}

func (s *Starting) OnAlive(source services.Instance) (State, error) {
	return s, nil
}

func (s *Starting) OnVictory(source services.Instance) (State, error) {
	return &NotElected{}, nil
}

// Starting election.

type StartingElection struct {
	elapsed  time.Duration
	services services.ServiceDiscovery
}

func (s *StartingElection) Tick(elapsed time.Duration) (State, error) {
	ok, err := s.services.MustBeLeader()
	if err != nil {
		return s, err
	}

	if ok {
		s.services.AnnounceLeadership()
		return &Elected{}, nil
	}

	s.services.StartElection()
	return &StartedElection{}, nil
}

func (s *StartingElection) OnElection(source services.Instance) (State, error) {
	return s, nil
}

func (s *StartingElection) OnAlive(source services.Instance) (State, error) {
	return s, nil
}

func (s *StartingElection) OnVictory(source services.Instance) (State, error) {
	return &NotElected{}, nil
}

// Started election.

type StartedElection struct {
	elapsed time.Duration
}

func (s *StartedElection) Tick(elapsed time.Duration) (State, error) {
	s.elapsed -= elapsed
	if s.elapsed <= 0 {
		return &Elected{}, nil
	}

	return s, nil
}

func (s *StartedElection) OnElection(source services.Instance) (State, error) {
	return s, nil
}

func (s *StartedElection) OnAlive(source services.Instance) (State, error) {
	// TODO: Fill WaitingForElection.
	return &WaitingForElection{}, nil
}

func (s *StartedElection) OnVictory(source services.Instance) (State, error) {
	return &NotElected{}, nil
}

// Elected.

type Elected struct{}

func (s *Elected) Tick(elapsed time.Duration) (State, error) {
	return s, nil
}

func (s *Elected) OnElection(source services.Instance) (State, error) {
	return &StartingElection{}, nil
}

func (s *Elected) OnAlive(source services.Instance) (State, error) {
	return s, nil
}

func (s *Elected) OnVictory(source services.Instance) (State, error) {
	return &NotElected{}, ErrTransition
}

// Waiting for election.

type WaitingForElection struct {
	elapsed time.Duration
}

func (s *WaitingForElection) Tick(elapsed time.Duration) (State, error) {
	s.elapsed -= elapsed
	if s.elapsed <= 0 {
		return &StartingElection{}, nil
	}

	return s, nil
}

func (s *WaitingForElection) OnElection(source services.Instance) (State, error) {
	return s, nil
}

func (s *WaitingForElection) OnAlive(source services.Instance) (State, error) {
	return s, nil
}

func (s *WaitingForElection) OnVictory(source services.Instance) (State, error) {
	return &NotElected{}, nil
}

// Not elected.

type NotElected struct {
	services services.ServiceDiscovery
}

func (s *NotElected) Tick(elapsed time.Duration) (State, error) {
	ok, err := s.services.PingLeader()
	if err != nil {
		return s, err
	}

	if !ok {
		return &StartingElection{}, nil
	}

	return s, nil
}

func (s *NotElected) OnElection(source services.Instance) (State, error) {
	return s, nil
}

func (s *NotElected) OnAlive(source services.Instance) (State, error) {
	return s, nil
}

func (s *NotElected) OnVictory(source services.Instance) (State, error) {
	return s, ErrTransition
}
