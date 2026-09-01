package redisclient

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/redis/go-redis/v9"

	"github.com/PandaX185/clinic-management/internal/platform/retry"
)

type Client = redis.Client

// New connects to Redis with configurable context + timeout + retry, then
// verifies reachability with a PING before returning. On success it returns
// a ready client; the caller decides how to handle an error (this package
// does not swallow failures or log on the caller's behalf).
func New(ctx context.Context, url string, rc retry.Config) (*Client, error) {
	opts, err := redis.ParseURL(url)
	if err != nil {
		return nil, fmt.Errorf("parse redis url: %w", err)
	}
	rc = retry.WithDefaults(rc)

	var client *redis.Client
	err = retry.Do(ctx, rc, func(ctx context.Context) error {
		c := redis.NewClient(opts)
		if err := c.Ping(ctx).Err(); err != nil {
			_ = c.Close()
			return err
		}
		client = c
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("connect redis (attempts=%d): %w", rc.Attempts, err)
	}
	return client, nil
}

// TryNew is a convenience for callers that treat Redis as optional (degraded
// mode): it returns a nil client and logs a warning instead of failing.
func TryNew(ctx context.Context, url string, rc retry.Config, log *slog.Logger) *Client {
	c, err := New(ctx, url, rc)
	if err != nil {
		log.Warn("redis unavailable; continuing degraded", "error", err.Error())
		return nil
	}
	return c
}
