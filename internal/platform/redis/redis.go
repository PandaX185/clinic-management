package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type Client struct {
	*redis.Client
}

func NewClient(redisURL string) (*Client, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("parse redis URL: %w", err)
	}

	fmt.Printf("DEBUG: Redis opts - Addr: %s, DB: %d, Password: %s\n", opts.Addr, opts.DB, opts.Password)

	client := redis.NewClient(opts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("ping redis: %w", err)
	}

	fmt.Printf("DEBUG: Redis ping successful\n")
	return &Client{client}, nil
}

// Ping wraps the embedded client's Ping and returns error
func (c *Client) Ping(ctx context.Context) error {
	return c.Client.Ping(ctx).Err()
}