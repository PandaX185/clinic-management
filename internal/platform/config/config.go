package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

type Config struct {
	Env  string `env:"ENV" envDefault:"development"`
	Port string `env:"PORT" envDefault:"8080"`

	LogLevel string `env:"LOG_LEVEL" envDefault:"info"`
	Format   string `env:"LOG_FORMAT" envDefault:"json"`

	DatabaseURL string `env:"DATABASE_URL,notEmpty"`
	RedisURL    string `env:"REDIS_URL" envDefault:"redis://localhost:6379"`
	NATSURL     string `env:"NATS_URL" envDefault:"nats://localhost:4222"`

	JWTSecret        string        `env:"JWT_SECRET,notEmpty"`
	JWTRefreshSecret string        `env:"JWT_REFRESH_SECRET,notEmpty"`
	AccessTokenTTL   time.Duration `env:"ACCESS_TOKEN_TTL" envDefault:"15m"`
	RefreshTokenTTL  time.Duration `env:"REFRESH_TOKEN_TTL" envDefault:"168h"`

	BcryptCost int `env:"BCRYPT_COST" envDefault:"12"`

	IdempotencyTTL     time.Duration `env:"IDEMPOTENCY_TTL" envDefault:"24h"`
	RateLimitPerMinute int           `env:"RATE_LIMIT_PER_MINUTE" envDefault:"60"`

	ReadTimeout    time.Duration `env:"READ_TIMEOUT" envDefault:"10s"`
	WriteTimeout   time.Duration `env:"WRITE_TIMEOUT" envDefault:"10s"`
	IdleTimeout    time.Duration `env:"IDLE_TIMEOUT" envDefault:"120s"`
	ShutdownPeriod time.Duration `env:"SHUTDOWN_PERIOD" envDefault:"30s"`
}

func Load() (Config, error) {
	return env.ParseAs[Config]()
}
