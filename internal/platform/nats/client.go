package natsclient

import (
	"context"
	"fmt"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/PandaX185/clinic-management/internal/platform/retry"
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

func New(ctx context.Context, url string, rc retry.Config) (*Client, error) {
	rc = retry.WithDefaults(rc)

	var conn *nats.Conn
	err := retry.Do(ctx, rc, func(ctx context.Context) error {
		c, err := nats.Connect(url,
			nats.MaxReconnects(-1),
			nats.ReconnectWait(2*time.Second),
			nats.Timeout(rc.Timeout),
		)
		if err != nil {
			return err
		}
		conn = c
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("connect nats (attempts=%d): %w", rc.Attempts, err)
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
