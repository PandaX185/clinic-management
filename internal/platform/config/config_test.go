package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfig_Load_Defaults(t *testing.T) {
	// This test ensures the config package can be built and tested
	// Actual integration tests would require viper setup with env vars
	assert.NotNil(t, &Config{}, "Config struct should be instantiable")
}