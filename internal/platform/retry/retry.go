// Package retry provides a small, configurable context/timeout/backoff
// primitive shared by all external connection bootstrap (Postgres, Redis,
// NATS) so timeouts and retry behaviour are operator-tunable, not hardcoded.
package retry

import (
	"context"
	"time"
)

// Config controls a single connect attempt's deadline and the retry schedule.
// A side-effect-free default is provided by DefaultConfig; embed it and only
// override fields the environment sets. The env tags let the struct be parsed
// by caarlos0/env when embedded in application config (see config.Config).
type Config struct {
	// Timeout bounds each individual attempt.
	Timeout time.Duration `env:"TIMEOUT" envDefault:"5s"`
	// Attempts is the total number of attempts (>=1).
	Attempts int `env:"ATTEMPTS" envDefault:"5"`
	// Backoff is the initial delay between attempts; it is doubled each time.
	Backoff time.Duration `env:"BACKOFF" envDefault:"500ms"`
}

// DefaultConfig mirrors the common-case values; use with the Defaults()
// helper or copy into your own struct with env-tagged defaults.
func DefaultConfig() Config {
	return Config{
		Timeout:  5 * time.Second,
		Attempts: 5,
		Backoff:  500 * time.Millisecond,
	}
}

// WithDefaults returns c with any zero-valued field filled from DefaultConfig.
func WithDefaults(c Config) Config {
	d := DefaultConfig()
	if c.Timeout <= 0 {
		c.Timeout = d.Timeout
	}
	if c.Attempts <= 0 {
		c.Attempts = d.Attempts
	}
	if c.Backoff <= 0 {
		c.Backoff = d.Backoff
	}
	return c
}

// Do runs fn until it succeeds, the parent context is done, or the attempt
// budget is exhausted. Each call runs under a per-attempt timeout derived from
// Config.Timeout plus the parent deadline.
func Do(ctx context.Context, cfg Config, fn func(ctx context.Context) error) error {
	cfg = WithDefaults(cfg)
	var err error
	delay := cfg.Backoff
	for i := 0; i < cfg.Attempts; i++ {
		if err = ctx.Err(); err != nil {
			return err
		}
		attemptCtx, cancel := context.WithTimeout(ctx, cfg.Timeout)
		err = fn(attemptCtx)
		cancel()
		if err == nil {
			return nil
		}
		if i == cfg.Attempts-1 {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
		delay *= 2
	}
	return err
}
