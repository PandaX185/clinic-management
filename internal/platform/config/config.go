package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds all runtime configuration, loaded exclusively from
// environment variables (see .env.example at the repo root).
type Config struct {
	Port             string        `env:"PORT" envDefault:"8080"`
	LogLevel         string        `env:"LOG_LEVEL" envDefault:"info"`
	DatabaseURL      string        `env:"DATABASE_URL,notEmpty"`
	RedisURL         string        `env:"REDIS_URL,notEmpty"`
	NATSURL          string        `env:"NATS_URL,notEmpty"`
	JWTSecret        string        `env:"JWT_SECRET,notEmpty"`
	JWTRefreshSecret string        `env:"JWT_REFRESH_SECRET,notEmpty"`
	JWTAccessTTL     time.Duration `env:"JWT_ACCESS_TTL" envDefault:"15m"`
	JWTRefreshTTL    time.Duration `env:"JWT_REFRESH_TTL" envDefault:"168h"`
}

func Load() (*Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return nil, err
	}
	return &cfg, nil
}
