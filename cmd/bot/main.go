package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
)

func main() {
	logger := newLogger(os.Getenv("BOT_ENV"), os.Getenv("BOT_LOG_LEVEL"))
	slog.SetDefault(logger)

	logger.Info("starting poly-don-bot")

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil && !errors.Is(err, context.Canceled) {
		logger.Error("bot exited with error", slog.Any("error", err))
		os.Exit(1)
	}

	logger.Info("bot shutdown complete")
}

func run(ctx context.Context, logger *slog.Logger) error {
	logger.Info("bot running")
	<-ctx.Done()
	logger.Info("shutdown signal received")
	return ctx.Err()
}

func newLogger(env, level string) *slog.Logger {
	var lvl slog.Level
	switch level {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}

	opts := &slog.HandlerOptions{Level: lvl}

	var handler slog.Handler
	if env == "prod" {
		handler = slog.NewJSONHandler(os.Stderr, opts)
	} else {
		handler = slog.NewTextHandler(os.Stderr, opts)
	}

	return slog.New(handler)
}
