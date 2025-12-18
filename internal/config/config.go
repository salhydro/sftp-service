package config

import (
	"fmt"
	"os"

	"github.com/joho/godotenv"
)

type Config struct {
	FuturAPIURL     string
	SFTPHostKeyPath string
	SFTPPort        string
}

// LoadConfig loads configuration from environment variables
func LoadConfig() (*Config, error) {
	// Try to load .env file (ignore error if file doesn't exist)
	_ = godotenv.Load()

	config := &Config{
		FuturAPIURL:     getEnv("FUTUR_API_URL", "http://localhost:3000/api/futur"),
		SFTPHostKeyPath: getEnv("SFTP_HOST_KEY_PATH", "./host_key"),
		SFTPPort:        getEnv("SFTP_PORT", "2222"),
	}

	// Validate required configuration
	if config.FuturAPIURL == "" {
		return nil, fmt.Errorf("FUTUR_API_URL is required")
	}

	return config, nil
}

func getEnv(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
