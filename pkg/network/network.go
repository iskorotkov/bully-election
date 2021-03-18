package network

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/ioutil"
	"net/http"
	"time"

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

type OutcomingMessage struct {
	Destination replicas.Replica
	Content     messages.Message
}

type Client struct {
	electionCh      chan IncomingMessage
	aliveCh         chan IncomingMessage
	victoryCh       chan IncomingMessage
	server          *http.Server
	shutdownTimeout time.Duration
	logger          *zap.Logger
}

func NewClient(shutdownTimeout time.Duration, logger *zap.Logger) *Client {
	electionCh := make(chan IncomingMessage)
	aliveCh := make(chan IncomingMessage)
	victoryCh := make(chan IncomingMessage)

	server := &http.Server{Addr: ":80"}

	go func() {
		logger := logger.Named("listener")

		http.HandleFunc("/", func(rw http.ResponseWriter, r *http.Request) {
			logger := logger.Named("handler")
			logger.Debug("incoming request", zap.Any("request", r))

			b, err := ioutil.ReadAll(r.Body)
			if err != nil {
				msg := "couldn't read request body"
				logger.Error(msg,
					zap.Error(err))
				http.Error(rw, msg, http.StatusBadRequest)
				return
			}

			var m messages.Message
			if err := json.Unmarshal(b, &m); err != nil {
				msg := "couldn't unamrshal request body"
				logger.Error(msg,
					zap.Error(err))
				http.Error(rw, msg, http.StatusBadRequest)
				return
			}

			origin := r.Header.Get("Origin")
			source := replicas.NewReplica(origin)

			switch m {
			case messages.MessageElection:
				electionCh <- IncomingMessage{source, m}
			case messages.MessageAlive:
				aliveCh <- IncomingMessage{source, m}
			case messages.MessageVictory:
				victoryCh <- IncomingMessage{source, m}
			default:
				logger.Warn("unknown message type",
					zap.Any("message", m))
			}
		})

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Warn("error in server occurred",
				zap.Error(err))
		}
	}()

	return &Client{
		electionCh:      electionCh,
		aliveCh:         aliveCh,
		victoryCh:       victoryCh,
		server:          server,
		shutdownTimeout: shutdownTimeout,
		logger:          logger,
	}
}

func (c *Client) Send(ctx context.Context, m OutcomingMessage) error {
	logger := c.logger.Named("send")

	b, err := json.Marshal(m.Content)
	if err != nil {
		logger.Warn("couldn't marshal message content",
			zap.Any("message", m),
			zap.Error(err))
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "post", m.Destination.Name, bytes.NewReader(b))
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

func (c *Client) OnElection() <-chan IncomingMessage {
	c.logger.Debug("election message received")
	return c.electionCh
}

func (c *Client) OnAlive() <-chan IncomingMessage {
	c.logger.Debug("alive message received")
	return c.aliveCh
}

func (c *Client) OnVictory() <-chan IncomingMessage {
	c.logger.Debug("victory message received")
	return c.victoryCh
}

func (c *Client) Close() error {
	logger := c.logger.Named("closed")
	logger.Debug("client closed")

	defer close(c.electionCh)
	defer close(c.aliveCh)
	defer close(c.victoryCh)

	ctx, cancel := context.WithTimeout(context.Background(), c.shutdownTimeout)
	defer cancel()

	if err := c.server.Shutdown(ctx); err != nil {
		logger.Warn("server shutdown failed",
			zap.Error(err))
		return err
	}

	return nil
}
