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
	From    replicas.Replica `json:"from"`
	Message messages.Message `json:"message"`
}

type OutgoingMessage struct {
	From    replicas.Replica
	To      replicas.Replica
	Message messages.Message
}

func NewOutgoingMessage(from replicas.Replica, to replicas.Replica, msg messages.Message) OutgoingMessage {
	return OutgoingMessage{
		From:    from,
		To:      to,
		Message: msg,
	}
}

type Server struct {
	electionCh chan IncomingMessage
	victoryCh  chan IncomingMessage
	logger     *zap.Logger
}

func NewServer(logger *zap.Logger) *Server {
	return &Server{
		electionCh: make(chan IncomingMessage),
		victoryCh:  make(chan IncomingMessage),
		logger:     logger,
	}
}

func (s *Server) Handle(rw http.ResponseWriter, r *http.Request) {
	logger := s.logger.Named("handler")
	logger.Debug("incoming request",
		zap.Any("header", r.Header))

	b, err := ioutil.ReadAll(r.Body)
	if err != nil {
		msg := "couldn't read request body"
		logger.Error(msg,
			zap.Error(err))
		http.Error(rw, msg, http.StatusBadRequest)
		return
	}

	var msg IncomingMessage
	if err := json.Unmarshal(b, &msg); err != nil {
		msg := "couldn't unamrshal request body"
		logger.Error(msg,
			zap.Error(err))
		http.Error(rw, msg, http.StatusBadRequest)
		return
	}

	logger.Debug("incoming message received and processed",
		zap.Any("message", msg))

	fmt.Fprint(rw, messages.MessageConfirm)

	switch msg.Message {
	case messages.MessageElection:
		logger.Debug("election request received",
			zap.Any("message", msg))
		s.electionCh <- msg
	case messages.MessageVictory:
		logger.Debug("victory message received",
			zap.Any("message", msg))
		s.victoryCh <- msg
	case messages.MessageAlive:
		logger.Debug("alive check received",
			zap.Any("message", msg))
	default:
		logger.Error("unknown message type",
			zap.Any("message", msg))
	}
}

func (s *Server) OnElectionRequest() <-chan IncomingMessage {
	return s.electionCh
}

func (s *Server) OnVictoryRequest() <-chan IncomingMessage {
	return s.victoryCh
}

func (s *Server) Close() {
	s.logger.Debug("comms server closed")
	close(s.electionCh)
	close(s.victoryCh)
}

type Client struct {
	aliveResponseCh    chan IncomingMessage
	electionResponseCh chan IncomingMessage
	logger             *zap.Logger
}

func NewClient(logger *zap.Logger) *Client {
	return &Client{
		aliveResponseCh:    make(chan IncomingMessage),
		electionResponseCh: make(chan IncomingMessage),
		logger:             logger,
	}
}

func (c *Client) Send(ctx context.Context, outgoing OutgoingMessage) error {
	logger := c.logger.Named("send")
	logger.Debug("starting sending message",
		zap.Any("message", outgoing))

	message := IncomingMessage{
		From:    outgoing.From,
		Message: outgoing.Message,
	}

	b, err := json.Marshal(message)
	if err != nil {
		logger.Error("couldn't marshal message content",
			zap.Any("message", outgoing),
			zap.Error(err))
		return err
	}

	url := fmt.Sprintf("http://%s", outgoing.To.IP)

	logger.Debug("sending message to url",
		zap.Any("message", outgoing),
		zap.String("url", url))

	req, err := http.NewRequestWithContext(ctx, "post", url, bytes.NewReader(b))
	if err != nil {
		logger.Error("couldn't create request",
			zap.Any("message", outgoing),
			zap.Error(err))
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Error("couldn't execute request",
			zap.Any("request", req),
			zap.Any("context", ctx),
			zap.Error(err))
		return err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			logger.Error("response body couldn't be closed", zap.Any("resp", resp), zap.Error(err))
		}
	}()

	if resp.StatusCode != http.StatusOK {
		logger.Error("message send failed with invalid response status code",
			zap.Int("code", resp.StatusCode))
		return ErrFailed
	}

	b, err = ioutil.ReadAll(resp.Body)
	if err != nil {
		logger.Error("couln't read response body",
			zap.Error(err))
		return err
	}

	var incoming IncomingMessage
	if err := json.Unmarshal(b, &incoming); err != nil {
		logger.Error("couldn't unmarshal response message",
			zap.Any("request", message),
			zap.Error(err))
		return err
	}

	logger.Debug("response received",
		zap.Any("response", incoming))

	switch outgoing.Message {
	case messages.MessageAlive:
		logger.Debug("alive response received",
			zap.Any("message", incoming))
		c.aliveResponseCh <- incoming
	case messages.MessageElection:
		logger.Debug("election response received",
			zap.Any("message", incoming))
		c.electionResponseCh <- incoming
	case messages.MessageVictory:
		logger.Debug("victory response received",
			zap.Any("message", incoming))
	}

	return nil
}

func (c *Client) OnAliveResponse() <-chan IncomingMessage {
	return c.aliveResponseCh
}

func (c *Client) OnElectionResponse() <-chan IncomingMessage {
	return c.electionResponseCh
}

func (c *Client) Close() {
	c.logger.Debug("comms client closed")
	close(c.aliveResponseCh)
	close(c.electionResponseCh)
}
