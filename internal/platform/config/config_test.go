package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requiredVars returns a full, valid set of the required env vars,
// so individual tests only override what they care about.
func requiredVars() map[string]string {
	return map[string]string{
		"DATABASE_URL":      "postgres://clinic:pw@localhost:5432/clinic?sslmode=disable",
		"REDIS_URL":         "redis://localhost:6379",
		"NATS_URL":          "nats://localhost:4222",
		"JWT_SECRET":        "test-access-secret",
		"JWT_REFRESH_SECRET": "test-refresh-secret",
	}
}

func setEnv(t *testing.T, vars map[string]string) {
	t.Helper()
	for k, v := range vars {
		t.Setenv(k, v)
	}
}

func TestConfig_Load_MissingRequired(t *testing.T) {
	tests := []struct {
		name    string
		dropKey string
	}{
		{"missing DATABASE_URL", "DATABASE_URL"},
		{"missing REDIS_URL", "REDIS_URL"},
		{"missing NATS_URL", "NATS_URL"},
		{"missing JWT_SECRET", "JWT_SECRET"},
		{"missing JWT_REFRESH_SECRET", "JWT_REFRESH_SECRET"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			vars := requiredVars()
			delete(vars, tt.dropKey)
			setEnv(t, vars)

			cfg, err := Load()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.dropKey)
			assert.Nil(t, cfg)
		})
	}
}

func TestConfig_Load_Defaults(t *testing.T) {
	setEnv(t, requiredVars())

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "8080", cfg.Port)
	assert.Equal(t, "info", cfg.LogLevel)
	assert.Equal(t, 15*time.Minute, cfg.JWTAccessTTL)
	assert.Equal(t, 168*time.Hour, cfg.JWTRefreshTTL)
}

func TestConfig_Load_ValidFullSet(t *testing.T) {
	vars := requiredVars()
	vars["PORT"] = "9090"
	vars["LOG_LEVEL"] = "debug"
	vars["JWT_ACCESS_TTL"] = "30m"
	vars["JWT_REFRESH_TTL"] = "720h"
	setEnv(t, vars)

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "9090", cfg.Port)
	assert.Equal(t, "debug", cfg.LogLevel)
	assert.Equal(t, vars["DATABASE_URL"], cfg.DatabaseURL)
	assert.Equal(t, vars["REDIS_URL"], cfg.RedisURL)
	assert.Equal(t, vars["NATS_URL"], cfg.NATSURL)
	assert.Equal(t, vars["JWT_SECRET"], cfg.JWTSecret)
	assert.Equal(t, vars["JWT_REFRESH_SECRET"], cfg.JWTRefreshSecret)
	assert.Equal(t, 30*time.Minute, cfg.JWTAccessTTL)
	assert.Equal(t, 720*time.Hour, cfg.JWTRefreshTTL)
}

func TestConfig_Load_InvalidDuration(t *testing.T) {
	vars := requiredVars()
	vars["JWT_ACCESS_TTL"] = "not-a-duration"
	setEnv(t, vars)

	cfg, err := Load()
	require.Error(t, err)
	assert.Nil(t, cfg)
}
