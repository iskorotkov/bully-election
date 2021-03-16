package states

type State interface {
	Execute() error
}

type Starting struct{}

type StartedElection struct{}

type Elected struct{}

type WaitingElection struct{}

type NotElected struct{}
