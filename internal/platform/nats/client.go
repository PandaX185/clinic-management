package natsclient

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
)

type Client struct {
	Conn  *nats.Conn
	Jet   jetstream.JetStream
	Topic string
}

const (
	StreamName    = "NOTIFICATIONS"
	SubjectNotify = "notifications.send"
	DLQSubject    = "notifications.dlq"
	ConsumerName  = "notification-worker"
)

func New(ctx context.Context, url string) (*Client, error) {
	conn, err := nats.Connect(url,
		nats.MaxReconnects(-1),
		nats.ReconnectWait(2*time.Second),
		nats.Timeout(5*time.Second),
	)
	if err != nil {
		return nil, fmt.Errorf("connect nats: %w", err)
	}

	js, err := jetstream.New(conn)
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create jetstream: %w", err)
	}

	stream, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:      StreamName,
		Subjects:  []string{SubjectNotify, DLQSubject},
		Retention: jetstream.WorkQueuePolicy,
	})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("create stream: %w", err)
	}
	_ = stream

	return &Client{Conn: conn, Jet: js}, nil
}

func (c *Client) Close() {
	if c.Conn != nil && !c.Conn.IsClosed() {
		c.Conn.Close()
	}
}
