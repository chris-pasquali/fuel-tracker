package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Host             string
	Port             string
	DatabaseURL      string
	RedisURL         string
	NSWFuelAPIKey    string
	NSWFuelAPISecret string
	CollectInterval  time.Duration
	SnapshotPath     string
	HistoryDays      int
	CORSOrigin       string
}

func Load() Config {
	return Config{
		Host:             env("HOST", "0.0.0.0"),
		Port:             env("PORT", "8080"),
		DatabaseURL:      env("DATABASE_URL", "postgres://fillwise:fillwise@localhost:5432/fillwise?sslmode=disable"),
		RedisURL:         env("REDIS_URL", "redis://127.0.0.1:6379"),
		NSWFuelAPIKey:    env("NSW_FUEL_API_KEY", "1MYSRAx5yvqHUZc6VGtxix6oMA2qgfRT"),
		NSWFuelAPISecret: env("NSW_FUEL_API_SECRET", "BMvWacw15Et8uFGF"),
		CollectInterval:  time.Duration(envInt("COLLECT_INTERVAL_MINUTES", 180)) * time.Minute,
		SnapshotPath:     env("SNAPSHOT_PATH", "data/sydney_snapshot.json"),
		HistoryDays:      envInt("HISTORY_DAYS", 90),
		CORSOrigin:       env("CORS_ORIGIN", "*"),
	}
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return fallback
}
