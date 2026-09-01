package database

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/PandaX185/clinic-management/internal/platform/retry"
)

type Pool = pgxpool.Pool

// New creates a Postgres connection pool, probing availability with
// configurable context + timeout + retry. The configured pool limits are
// applied on top of the connection string, if any.
func New(ctx context.Context, url string, rc retry.Config) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		return nil, fmt.Errorf("parse database url: %w", err)
	}
	cfg.MaxConns = 20
	cfg.MinConns = 2
	cfg.MaxConnLifetime = time.Hour
	cfg.MaxConnIdleTime = 30 * time.Minute

	rc = retry.WithDefaults(rc)

	var pool *pgxpool.Pool
	err = retry.Do(ctx, rc, func(ctx context.Context) error {
		p, err := pgxpool.NewWithConfig(ctx, cfg)
		if err != nil {
			return err
		}
		if err := p.Ping(ctx); err != nil {
			p.Close()
			return err
		}
		pool = p
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("connect database (attempts=%d): %w", rc.Attempts, err)
	}
	return pool, nil
}
