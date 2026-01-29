package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

// Config holds the application configuration.
type Config struct {
	Token        string
	SerialNumber string
}

// Load reads configuration from a .env file (if present) and environment variables.
// Environment variables take precedence over .env values.
func Load() (*Config, error) {
	// godotenv.Load does not overwrite existing env vars
	_ = godotenv.Load()

	token := os.Getenv("VICO_TOKEN")
	if token == "" {
		return nil, fmt.Errorf("VICO_TOKEN environment variable is required")
	}

	sn := os.Getenv("VICO_SN")
	if sn == "" {
		return nil, fmt.Errorf("VICO_SN environment variable is required")
	}

	return &Config{
		Token:        token,
		SerialNumber: sn,
	}, nil
}
