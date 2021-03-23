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

type Request struct {
	From    replicas.Replica `json:"from,omitempty"`
	Message messages.Message `json:"message"`
}

type Server struct {
	electionCh chan Request
	victoryCh  chan Request
	logger     *zap.Logger
}

func NewServer(logger *zap.Logger) *Server {
	return &Server{
		electionCh: make(chan Request),
		victoryCh:  make(chan Request),
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

	var request Request
	if err := json.Unmarshal(b, &request); err != nil {
		msg := "couldn't unmarshal request body"
		logger.Error(msg,
			zap.ByteString("request", b),
			zap.Error(err))
		http.Error(rw, msg, http.StatusBadRequest)
		return
	}

	logger.Debug("incoming message received and processed",
		zap.Any("message", request))

	response := Request{
		Message: messages.MessageConfirm,
	}

	b, err = json.Marshal(response)
	if err != nil {
		msg := "couldn't marshal response body"
		logger.Error(msg,
			zap.Any("response", response),
			zap.Error(err))
		return
	}

	fmt.Fprint(rw, string(b))

	switch request.Message {
	case messages.MessageElection:
		logger.Debug("election request received",
			zap.Any("message", request))
		s.electionCh <- request
	case messages.MessageVictory:
		logger.Debug("victory message received",
			zap.Any("message", request))
		s.victoryCh <- request
	case messages.MessageAlive:
		logger.Debug("alive check received",
			zap.Any("message", request))
	default:
		logger.Error("unknown message type",
			zap.Any("message", request))
	}
}

func (s *Server) OnElectionRequest() <-chan Request {
	return s.electionCh
}

func (s *Server) OnVictoryRequest() <-chan Request {
	return s.victoryCh
}

func (s *Server) Close() {
	s.logger.Debug("comms server closed")
	close(s.electionCh)
	close(s.victoryCh)
}

type Client struct {
	aliveResponseCh    chan Request
	electionResponseCh chan Request
	logger             *zap.Logger
}

func NewClient(logger *zap.Logger) *Client {
	return &Client{
		aliveResponseCh:    make(chan Request),
		electionResponseCh: make(chan Request),
		logger:             logger,
	}
}

func (c *Client) Send(ctx context.Context, request Request, to replicas.Replica) error {
	logger := c.logger.Named("send")
	logger.Debug("starting sending message",
		zap.Any("request", request),
		zap.Any("to", to))

	b, err := json.Marshal(request)
	if err != nil {
		logger.Error("couldn't marshal message content",
			zap.Any("request", request),
			zap.Any("to", to),
			zap.Error(err))
		return err
	}

	url := fmt.Sprintf("http://%s", to.IP)

	logger.Debug("sending message to url",
		zap.Any("request", request),
		zap.Any("to", to),
		zap.String("url", url))

	req, err := http.NewRequestWithContext(ctx, "post", url, bytes.NewReader(b))
	if err != nil {
		logger.Error("couldn't create request",
			zap.Any("request", request),
			zap.Any("to", to),
			zap.Error(err))
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		logger.Error("couldn't execute request",
			zap.Any("request", request),
			zap.Any("to", to),
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

	var response Request
	if err := json.Unmarshal(b, &response); err != nil {
		logger.Error("couldn't unmarshal response message",
			zap.Any("request", request),
			zap.ByteString("response", b),
			zap.Error(err))
		return err
	}

	logger.Debug("response message received",
		zap.Any("response", response))

	switch request.Message {
	case messages.MessageAlive:
		logger.Debug("alive response received",
			zap.Any("message", response))
		c.aliveResponseCh <- response
	case messages.MessageElection:
		logger.Debug("election response received",
			zap.Any("message", response))
		c.electionResponseCh <- response
	case messages.MessageVictory:
		logger.Debug("victory response received",
			zap.Any("message", response))
	}

	return nil
}

func (c *Client) OnAliveResponse() <-chan Request {
	return c.aliveResponseCh
}

func (c *Client) OnElectionResponse() <-chan Request {
	return c.electionResponseCh
}

func (c *Client) Close() {
	c.logger.Debug("comms client closed")
	close(c.aliveResponseCh)
	close(c.electionResponseCh)
}
