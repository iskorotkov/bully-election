package network

import (
	"bytes"
	"context"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/iskorotkov/bully-election/pkg/messages"
	"go.uber.org/zap"
)

type IncomingMessage struct {
	Source  string
	Content messages.Message
}

type OutcomingMessage struct {
	Destination string
	Content     messages.Message
}

type Client struct {
	electionCh chan IncomingMessage
	aliveCh    chan IncomingMessage
	victoryCh  chan IncomingMessage
	closeCh    chan struct{}
	logger     *zap.Logger
}

func NewClient(logger *zap.Logger) *Client {
	electionCh := make(chan IncomingMessage)
	aliveCh := make(chan IncomingMessage)
	victoryCh := make(chan IncomingMessage)
	closeCh := make(chan struct{})

	go func() {
		logger := logger.Named("listener")
		for {
			select {
			case <-closeCh:
				close(electionCh)
				close(aliveCh)
				close(victoryCh)
				return
			default:
				http.ListenAndServe(":80", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
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

					switch m {
					case messages.MessageElection:
						electionCh <- IncomingMessage{origin, m}
					case messages.MessageAlive:
						aliveCh <- IncomingMessage{origin, m}
					case messages.MessageVictory:
						victoryCh <- IncomingMessage{origin, m}
					default:
						logger.Warn("unknown message type",
							zap.Any("message", m))
					}
				}))
			}
		}
	}()

	return &Client{
		electionCh: electionCh,
		aliveCh:    aliveCh,
		victoryCh:  victoryCh,
		closeCh:    closeCh,
		logger:     logger,
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

	req, err := http.NewRequestWithContext(ctx, "post", m.Destination, bytes.NewReader(b))
	if err != nil {
		logger.Warn("couldn't create request",
			zap.Any("message", m),
			zap.String("destination", m.Destination),
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
	c.logger.Debug("client closed")
	c.closeCh <- struct{}{}
	close(c.closeCh)
	return nil
}
