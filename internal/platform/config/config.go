package config

import (
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
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	viper.AddConfigPath("./config")
	viper.AddConfigPath("/etc/clinic")

	// Environment variable overrides
	viper.AutomaticEnv()
	viper.SetEnvPrefix("CLINIC")

	// Defaults for local development
	viper.SetDefault("port", "8080")
	viper.SetDefault("log_level", "info")
	viper.SetDefault("database_url", "postgres://clinic:clinic@localhost:5432/clinic?sslmode=disable")
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
	}

	var cfg Config
	if err := viper.Unmarshal(&cfg); err != nil {
		return nil, err
	}

	return &cfg, nil
}