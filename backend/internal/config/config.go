// Package config loads the server configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"strconv"
)

// Config is the validated server configuration.
type Config struct {
	// Addr is the TCP address the HTTP server listens on, e.g. ":8080".
	Addr string
	// DatabaseURL is the Postgres connection string.
	DatabaseURL string
}

const defaultPort = 8080

// Load builds a Config from getenv, which is os.Getenv in production and a
// map lookup in tests.
func Load(getenv func(string) string) (Config, error) {
	databaseURL := getenv("DATABASE_URL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	port := defaultPort
	if raw := getenv("PORT"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 65535 {
			return Config{}, fmt.Errorf("PORT must be a number from 1 to 65535, got %q", raw)
		}
		port = parsed
	}

	return Config{
		Addr:        fmt.Sprintf(":%d", port),
		DatabaseURL: databaseURL,
	}, nil
}
