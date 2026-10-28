package platform

import (
	"errors"
	"os"
)

type Config struct {
	Environment string
	DatabaseURL string
}

func LoadConfig() (Config, error) {
	c := Config{Environment: os.Getenv("APP_ENV"), DatabaseURL: os.Getenv("DATABASE_URL")}
	if c.Environment != "development" && c.Environment != "production" {
		return Config{}, errors.New("APP_ENV must be development or production")
	}
	if c.DatabaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}
	return c, nil
}
