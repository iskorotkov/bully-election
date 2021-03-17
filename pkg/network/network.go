package network

import (
	"bytes"
	"context"
	"encoding/json"
	"log"
	"net/http"

	"github.com/iskorotkov/bully-election/pkg/messages"
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
}

func NewClient() *Client {
	electionCh := make(chan IncomingMessage)
	aliveCh := make(chan IncomingMessage)
	victoryCh := make(chan IncomingMessage)
	closeCh := make(chan struct{})

	go func() {
		for {
			select {
			case <-closeCh:
				close(electionCh)
				close(aliveCh)
				close(victoryCh)
				return
			default:
				http.ListenAndServe(":80", http.HandlerFunc(func(rw http.ResponseWriter, r *http.Request) {
					// TODO: Implement HTTP handler.
				}))
			}
		}
	}()

	return &Client{
		electionCh: electionCh,
		aliveCh:    aliveCh,
		victoryCh:  victoryCh,
		closeCh:    closeCh,
	}
}

func (c *Client) Send(ctx context.Context, m OutcomingMessage) error {
	b, err := json.Marshal(m.Content)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "post", m.Destination, bytes.NewReader(b))
	if err != nil {
		return err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Println(err)
		}
	}()

	return nil
}

func (c *Client) OnElection() <-chan IncomingMessage {
	return c.electionCh
}

func (c *Client) OnAlive() <-chan IncomingMessage {
	return c.aliveCh
}

func (c *Client) OnVictory() <-chan IncomingMessage {
	return c.victoryCh
}

func (c *Client) Close() error {
	c.closeCh <- struct{}{}
	close(c.closeCh)
	return nil
}
