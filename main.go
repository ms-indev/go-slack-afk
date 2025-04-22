package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/joho/godotenv"
	"github.com/pyama86/slack-afk/go/slack"
	"github.com/pyama86/slack-afk/go/store"
)

func validateEnv() error {
	requiredEnv := []string{
		"SLACK_BOT_TOKEN",
		"SLACK_APP_TOKEN",
	}
	for _, env := range requiredEnv {
		if os.Getenv(env) == "" {
			return fmt.Errorf("environment variable %s is not set", env)
		}
	}
	return nil
}

func main() {
	if _, err := os.Stat(".env"); err == nil {
		if err := godotenv.Load(); err != nil {
			slog.Error("Failed to load .env file", slog.Any("error", err))
			os.Exit(1)
		}
	}

	if err := validateEnv(); err != nil {
		slog.Error("Environment validation failed", slog.Any("error", err))
		os.Exit(1)
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}

	redisClient, err := store.NewRedisClient(redisURL)
	if err != nil {
		slog.Error("Failed to initialize Redis client", slog.Any("error", err))
		os.Exit(1)
	}

	slog.Info("Starting Slack bot...")
	if err := slack.StartSocketModeServer(redisClient); err != nil {
		slog.Error("Failed to start server", slog.Any("error", err))
		os.Exit(1)
	}
}
