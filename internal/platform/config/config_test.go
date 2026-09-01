package config

import (
	"testing"
	"time"
)

func TestLoad_ParsesConnectionRetryConfig(t *testing.T) {
	base := map[string]string{
		"DATABASE_URL":       "postgres://u:p@h:5432/db",
		"JWT_SECRET":         "secret",
		"JWT_REFRESH_SECRET": "secret2",
	}
	for k, v := range base {
		t.Setenv(k, v)
	}
	// Override connective tissue, leaving concurrency-safety untouched.
	t.Setenv("DB_CONNECT_TIMEOUT", "3s")
	t.Setenv("DB_CONNECT_ATTEMPTS", "7")
	t.Setenv("DB_CONNECT_BACKOFF", "250ms")
	t.Setenv("REDIS_CONNECT_ATTEMPTS", "9")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}

	if cfg.DBConnect.Timeout != 3*time.Second {
		t.Errorf("DB timeout = %v, want 3s", cfg.DBConnect.Timeout)
	}
	if cfg.DBConnect.Attempts != 7 {
		t.Errorf("DB attempts = %d, want 7", cfg.DBConnect.Attempts)
	}
	if cfg.DBConnect.Backoff != 250*time.Millisecond {
		t.Errorf("DB backoff = %v, want 250ms", cfg.DBConnect.Backoff)
	}
	if cfg.RedisConnect.Attempts != 9 {
		t.Errorf("Redis attempts = %d, want 9", cfg.RedisConnect.Attempts)
	}
}
