package config

import (
	"fmt"
	"os"
	"github.com/spf13/viper"
)

type Config struct {
	Port              string `mapstructure:"port"`
	LogLevel          string `mapstructure:"log_level"`
	DatabaseURL       string `mapstructure:"database_url"`
	RedisURL          string `mapstructure:"redis_url"`
	NATSURL           string `mapstructure:"nats_url"`
	JWTSecret         string `mapstructure:"jwt_secret"`
	JWTRefreshSecret  string `mapstructure:"jwt_refresh_secret"`
	JWTAccessTTL      string `mapstructure:"jwt_access_ttl"`
	JWTRefreshTTL     string `mapstructure:"jwt_refresh_ttl"`
}

func Load() (*Config, error) {
	// Check env vars directly first
	fmt.Printf("DEBUG: os.Getenv(AXIOM_DATABASE_URL) = %s\n", os.Getenv("AXIOM_DATABASE_URL"))
	fmt.Printf("DEBUG: os.Getenv(AXIOM_REDIS_URL) = %s\n", os.Getenv("AXIOM_REDIS_URL"))
	fmt.Printf("DEBUG: os.Getenv(AXIOM_NATS_URL) = %s\n", os.Getenv("AXIOM_NATS_URL"))

	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/axiom")

	// Environment variable overrides
	viper.AutomaticEnv()
	viper.SetEnvPrefix("AXIOM")

	// Defaults for local development
	viper.SetDefault("port", "8080")
	viper.SetDefault("log_level", "info")
	viper.SetDefault("database_url", "postgres://axiom:axiom@localhost:5432/axiom?sslmode=disable")
	viper.SetDefault("redis_url", "redis://localhost:6379")
	viper.SetDefault("nats_url", "nats://localhost:4222")
	viper.SetDefault("jwt_secret", "dev-secret-change-in-production")
	viper.SetDefault("jwt_refresh_secret", "dev-refresh-secret-change-in-production")
	viper.SetDefault("jwt_access_ttl", "15m")
	viper.SetDefault("jwt_refresh_ttl", "168h")

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil, err
		}
		fmt.Printf("DEBUG: Config file not found, using defaults and env vars\n")
	} else {
		fmt.Printf("DEBUG: Config file loaded from: %s\n", viper.ConfigFileUsed())
	}

	// Debug: Print all viper settings
	fmt.Printf("DEBUG: viper.AllSettings() = %+v\n", viper.AllSettings())

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	fmt.Printf("DEBUG: Loaded config - Port: %s, DB: %s, Redis: %s, NATS: %s\n", cfg.Port, cfg.DatabaseURL, cfg.RedisURL, cfg.NATSURL)

	return &cfg, nil
}