package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/adonmuhammaddd/poly-don-bot/internal/api"
	"github.com/adonmuhammaddd/poly-don-bot/internal/config"
	"github.com/adonmuhammaddd/poly-don-bot/internal/feeds/binance"
	"github.com/adonmuhammaddd/poly-don-bot/internal/feeds/polymarket"
	"github.com/adonmuhammaddd/poly-don-bot/internal/latency"
	"github.com/adonmuhammaddd/poly-don-bot/internal/observability"
	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
)

func main() {
	if err := run(); err != nil {
		_, _ = fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	logger := observability.NewLogger(cfg.Env, cfg.LogLevel)
	slog.SetDefault(logger)
	logger.Info("starting poly-don-bot",
		slog.String("env", cfg.Env),
		slog.String("log_level", cfg.LogLevel),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := postgres.NewPool(ctx, cfg.Postgres.URL)
	if err != nil {
		return fmt.Errorf("connect postgres: %w", err)
	}
	defer pool.Close()
	logger.Info("postgres connected")

	metrics := observability.NewMetrics()
	metricsServer := observability.NewMetricsServer(cfg.HTTP.MetricsPort, metrics)

	queries := postgres.New(pool)

	tracker := latency.NewTracker(latency.Config{})

	binanceClient := binance.NewClient(
		binance.Config{
			WSURL:             cfg.Binance.WSURL,
			Symbols:           cfg.Binance.Symbols,
			Streams:           cfg.Binance.Streams,
			InitialBackoff:    cfg.Binance.InitialBackoff,
			MaxBackoff:        cfg.Binance.MaxBackoff,
			HeartbeatInterval: cfg.Binance.HeartbeatInterval,
			HeartbeatTimeout:  cfg.Binance.HeartbeatTimeout,
		},
		logger,
		metrics,
		queries,
	)
	binanceClient.WithListener(tracker)

	polymarketClient := polymarket.NewClient(
		polymarket.Config{
			WSURL:             cfg.Polymarket.WSURL,
			RESTBaseURL:       cfg.Polymarket.RESTBaseURL,
			SlugPrefix:        cfg.Polymarket.SlugPrefix,
			RequestsPerMinute: cfg.Polymarket.RequestsPerMinute,
			InitialBackoff:    cfg.Polymarket.InitialBackoff,
			MaxBackoff:        cfg.Polymarket.MaxBackoff,
			PingInterval:      cfg.Polymarket.PingInterval,
			ReadTimeout:       cfg.Polymarket.ReadTimeout,
		},
		logger,
		metrics,
		queries,
	)
	polymarketClient.WithListener(tracker)

	apiServer := api.NewServer(api.Config{Port: cfg.HTTP.Port}, queries, tracker, logger)

	var wg sync.WaitGroup
	errs := make(chan error, 4)

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("metrics server listening", slog.Int("port", cfg.HTTP.MetricsPort))
		if err := metricsServer.Run(ctx); err != nil {
			errs <- fmt.Errorf("metrics server: %w", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := binanceClient.Run(ctx); err != nil {
			errs <- fmt.Errorf("binance feed: %w", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := polymarketClient.Run(ctx); err != nil {
			errs <- fmt.Errorf("polymarket feed: %w", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("api server listening", slog.Int("port", cfg.HTTP.Port))
		if err := apiServer.Run(ctx); err != nil {
			errs <- fmt.Errorf("api server: %w", err)
		}
	}()

	<-ctx.Done()
	logger.Info("shutdown signal received, draining...")

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		logger.Info("drained cleanly")
	case <-time.After(30 * time.Second):
		logger.Warn("drain timeout exceeded, forcing exit")
	}

	close(errs)
	var firstErr error
	for e := range errs {
		if firstErr == nil && !errors.Is(e, context.Canceled) {
			firstErr = e
		}
	}
	logger.Info("bot shutdown complete")
	return firstErr
}
