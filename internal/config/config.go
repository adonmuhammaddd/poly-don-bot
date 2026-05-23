package config

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env      string
	LogLevel string
	HTTP     HTTPConfig
	Postgres PostgresConfig
	Redis    RedisConfig
	Binance  BinanceConfig
}

type HTTPConfig struct {
	Port        int
	MetricsPort int
}

type PostgresConfig struct {
	URL string
}

type RedisConfig struct {
	URL string
}

type BinanceConfig struct {
	WSURL             string
	Symbols           []string
	Streams           []string
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
}

func Load() (Config, error) {
	var errs []error

	pgURL := os.Getenv("POSTGRES_URL")
	if pgURL == "" {
		errs = append(errs, errors.New("POSTGRES_URL is required"))
	}

	httpPort, err := envInt("BOT_HTTP_PORT", 8080)
	if err != nil {
		errs = append(errs, err)
	}
	metricsPort, err := envInt("BOT_METRICS_PORT", 9100)
	if err != nil {
		errs = append(errs, err)
	}

	c := Config{
		Env:      envStr("BOT_ENV", "dev"),
		LogLevel: envStr("BOT_LOG_LEVEL", "info"),
		HTTP: HTTPConfig{
			Port:        httpPort,
			MetricsPort: metricsPort,
		},
		Postgres: PostgresConfig{URL: pgURL},
		Redis: RedisConfig{
			URL: envStr("REDIS_URL", "redis://localhost:6379/0"),
		},
		Binance: BinanceConfig{
			WSURL:             envStr("BINANCE_WS_URL", "wss://stream.binance.com:9443/stream"),
			Symbols:           splitCSV(envStr("BINANCE_SYMBOLS", "btcusdt")),
			Streams:           []string{"aggTrade"},
			InitialBackoff:    time.Second,
			MaxBackoff:        30 * time.Second,
			HeartbeatInterval: 30 * time.Second,
			HeartbeatTimeout:  10 * time.Second,
		},
	}

	return c, errors.Join(errs...)
}

func envStr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def, fmt.Errorf("%s must be int: %w", key, err)
	}
	return n, nil
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	for i, p := range parts {
		parts[i] = strings.TrimSpace(p)
	}
	return parts
}
