package config

import (
	"fmt"
	"os"
	"time"
)

type Config struct {
	HTTPAddr          string
	DatabaseUrl       string
	ShutDownTimeout   time.Duration
	FirebaseProjectID string
}

func Load() (Config, error) {
	cfg := Config{
		HTTPAddr:          getenv("HTTP_ADDR", ":8080"),
		DatabaseUrl:       os.Getenv("DATABASE_URL"),
		ShutDownTimeout:   10 * time.Second,
		FirebaseProjectID: os.Getenv("FIREBASE_PROJECT_ID"),
	}

	if cfg.DatabaseUrl == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}

	if cfg.FirebaseProjectID == "" {
		return Config{}, fmt.Errorf("FIREBASE_PROJECT_ID is required")
	}
	return cfg, nil
}

func getenv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
