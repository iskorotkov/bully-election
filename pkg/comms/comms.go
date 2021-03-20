package comms

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/iskorotkov/bully-election/pkg/messages"
	"github.com/iskorotkov/bully-election/pkg/replicas"
	"go.uber.org/zap"
)

var (
	ErrFailed = errors.New("request send failed")
)

type IncomingMessage struct {
	Source  replicas.Replica
	Content messages.Message
}

type OutgoingMessage struct {
	Destination replicas.Replica
	Content     messages.Message
}

type Server struct {
	electionCh chan IncomingMessage
	aliveCh    chan IncomingMessage
	victoryCh  chan IncomingMessage
	logger     *zap.Logger
}

func NewServer(logger *zap.Logger) *Server {
	electionCh := make(chan IncomingMessage)
	aliveCh := make(chan IncomingMessage)
	victoryCh := make(chan IncomingMessage)

	return &Server{
		electionCh: electionCh,
		aliveCh:    aliveCh,
		victoryCh:  victoryCh,
		logger:     logger,
	}
}

func (s *Server) Handle(rw http.ResponseWriter, r *http.Request) {
	logger := s.logger.Named("handler")
	logger.Debug("incoming request", zap.Any("request", r))

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		msg := "couldn't read request body"
		logger.Error(msg,
			zap.Error(err))
		http.Error(rw, msg, http.StatusBadRequest)
		return
	}

	var msg messages.Message
	if err := json.Unmarshal(b, &msg); err != nil {
		msg := "couldn't unamrshal request body"
		logger.Error(msg,
			zap.Error(err))
		http.Error(rw, msg, http.StatusBadRequest)
		return
	}

	origin := r.Header.Get("Origin")
	source := replicas.NewReplica(origin)

	switch msg {
	case messages.MessageElection:
		s.electionCh <- IncomingMessage{source, msg}
	case messages.MessageAlive:
		s.aliveCh <- IncomingMessage{source, msg}
	case messages.MessageVictory:
		s.victoryCh <- IncomingMessage{source, msg}
	case messages.MessagePing:
		logger.Debug("server was pinged")
	default:
		logger.Warn("unknown message type",
			zap.Any("message", msg))
	}
}

func (s *Server) OnElection() <-chan IncomingMessage {
	s.logger.Debug("election message received")
	return s.electionCh
}

func (s *Server) OnAlive() <-chan IncomingMessage {
	s.logger.Debug("alive message received")
	return s.aliveCh
}

func (s *Server) OnVictory() <-chan IncomingMessage {
	s.logger.Debug("victory message received")
	return s.victoryCh
}

func (s *Server) Close() {
	defer close(s.electionCh)
	defer close(s.aliveCh)
	defer close(s.victoryCh)
}

type Client struct {
	logger *zap.Logger
}

func NewClient(logger *zap.Logger) *Client {
	return &Client{
		logger: logger,
	}
}

func (c *Client) Send(ctx context.Context, m OutgoingMessage) error {
	logger := c.logger.Named("send")

	b, err := json.Marshal(m.Content)
	if err != nil {
		logger.Warn("couldn't marshal message content",
			zap.Any("message", m),
			zap.Error(err))
		return err
	}

	url := fmt.Sprintf("http://%s", m.Destination.Name)

	req, err := http.NewRequestWithContext(ctx, "post", url, bytes.NewReader(b))
	if err != nil {
		logger.Warn("couldn't create request",
			zap.Any("message", m),
			zap.Any("destination", m.Destination),
			zap.Error(err))
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Warn("couldn't execute request",
			zap.Any("request", req),
			zap.Error(err))
		return err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Warn("response body couldn't be closed", zap.Any("resp", resp), zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return ErrFailed
	}

	return nil
}
