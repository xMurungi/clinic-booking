package config

import (
	// "fmt"
	"fmt"
	"os"
	"time"
)

type Config struct {
	HTTPAddr        string
	DatabaseUrl     string
	ShutDownTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:        getenv("HTTP_ADDR", ":8080"),
		DatabaseUrl:     os.Getenv("DATABASE_URL"),
		ShutDownTimeout: 10 * time.Second,
	}

	if cfg.DatabaseUrl == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	addr := os.Getenv("HTTP_ADDR")
	if addr == "" {
		if p := os.Getenv("PORT"); p != "" {
			addr = ":" + p
		} else {
			addr = ":8080"
		}
	}
	// then: HTTPAddr: addr,

	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
