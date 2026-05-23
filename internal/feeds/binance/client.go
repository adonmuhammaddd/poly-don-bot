package binance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/adonmuhammaddd/poly-don-bot/internal/observability"
	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
)

const exchangeName = "binance"

type Config struct {
	WSURL             string
	Symbols           []string
	Streams           []string
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	HeartbeatInterval time.Duration
	HeartbeatTimeout  time.Duration
}

// Storage is what the feed needs from the DB layer. Defined as an interface
// so tests can swap in a fake without touching pgx.
type Storage interface {
	InsertPriceTick(ctx context.Context, arg postgres.InsertPriceTickParams) (int64, error)
}

// Dialer abstracts websocket.DefaultDialer for tests.
type Dialer interface {
	DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (*websocket.Conn, *http.Response, error)
}

type Client struct {
	cfg     Config
	logger  *slog.Logger
	metrics *observability.Metrics
	storage Storage
	dialer  Dialer
}

func NewClient(cfg Config, logger *slog.Logger, metrics *observability.Metrics, storage Storage) *Client {
	return &Client{
		cfg:     cfg,
		logger:  logger.With(slog.String("component", "binance_feed")),
		metrics: metrics,
		storage: storage,
		dialer:  websocket.DefaultDialer,
	}
}

// WithDialer overrides the WebSocket dialer (used in tests).
func (c *Client) WithDialer(d Dialer) *Client {
	c.dialer = d
	return c
}

// Run blocks until ctx is cancelled. Reconnects with exponential backoff on errors.
func (c *Client) Run(ctx context.Context) error {
	backoff := c.cfg.InitialBackoff
	for ctx.Err() == nil {
		disconnectAt, runErr := c.runOnce(ctx)
		if ctx.Err() != nil {
			return nil
		}
		if runErr != nil {
			c.logger.Warn("connection ended", slog.Any("error", runErr))
		}

		c.logger.Info("waiting before reconnect", slog.Duration("backoff", backoff))
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}

		c.metrics.BinanceReconnectSeconds.Observe(time.Since(disconnectAt).Seconds())

		backoff *= 2
		if backoff > c.cfg.MaxBackoff {
			backoff = c.cfg.MaxBackoff
		}
	}
	return nil
}

// runOnce establishes a single WS session and processes messages until error.
// Returns the disconnect timestamp for reconnect-duration metrics.
func (c *Client) runOnce(ctx context.Context) (time.Time, error) {
	streamURL, err := buildStreamURL(c.cfg.WSURL, c.cfg.Symbols, c.cfg.Streams)
	if err != nil {
		return time.Now(), fmt.Errorf("build url: %w", err)
	}
	c.logger.Info("dialing", slog.String("url", streamURL))

	conn, resp, err := c.dialer.DialContext(ctx, streamURL, http.Header{})
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		c.metrics.BinanceDisconnects.WithLabelValues("dial_failed").Inc()
		return time.Now(), fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	c.logger.Info("connected")

	readDeadline := func() time.Time {
		return time.Now().Add(c.cfg.HeartbeatInterval + c.cfg.HeartbeatTimeout)
	}
	_ = conn.SetReadDeadline(readDeadline())
	conn.SetPongHandler(func(string) error {
		return conn.SetReadDeadline(readDeadline())
	})
	conn.SetPingHandler(func(appData string) error {
		_ = conn.SetReadDeadline(readDeadline())
		return conn.WriteControl(
			websocket.PongMessage,
			[]byte(appData),
			time.Now().Add(c.cfg.HeartbeatTimeout),
		)
	})

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	// Close the conn when ctx is cancelled so the blocking ReadMessage below returns.
	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	go func() {
		t := time.NewTicker(c.cfg.HeartbeatInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := conn.WriteControl(
					websocket.PingMessage,
					nil,
					time.Now().Add(c.cfg.HeartbeatTimeout),
				); err != nil {
					return
				}
			}
		}
	}()

	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			c.metrics.BinanceDisconnects.WithLabelValues(classifyError(err)).Inc()
			return time.Now(), fmt.Errorf("read: %w", err)
		}
		if err := c.handleMessage(ctx, raw); err != nil {
			c.logger.Warn("handle message failed", slog.Any("error", err))
		}
	}
}

func (c *Client) handleMessage(ctx context.Context, raw []byte) error {
	var msg CombinedStreamMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}

	if !strings.HasSuffix(msg.Stream, "@aggTrade") {
		return nil
	}

	var event AggTradeEvent
	if err := json.Unmarshal(msg.Data, &event); err != nil {
		return fmt.Errorf("unmarshal aggTrade: %w", err)
	}

	tradeTime := event.TradeTime()
	c.metrics.BinanceMessageLatency.Observe(time.Since(tradeTime).Seconds())

	price, err := event.PriceDecimal()
	if err != nil {
		return fmt.Errorf("parse price: %w", err)
	}

	symbol := strings.ToLower(event.Symbol)
	params := postgres.InsertPriceTickParams{
		Exchange:   exchangeName,
		Symbol:     symbol,
		Price:      price,
		TsExchange: pgtype.Timestamptz{Time: tradeTime, Valid: true},
		RawPayload: msg.Data,
	}
	if _, err := c.storage.InsertPriceTick(ctx, params); err != nil {
		return fmt.Errorf("insert: %w", err)
	}
	c.metrics.BinanceTicks.WithLabelValues(symbol, "aggTrade").Inc()
	return nil
}

func buildStreamURL(base string, symbols, streams []string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	out := make([]string, 0, len(symbols)*len(streams))
	for _, sym := range symbols {
		for _, stream := range streams {
			out = append(out, fmt.Sprintf("%s@%s", strings.ToLower(sym), stream))
		}
	}
	q := u.Query()
	q.Set("streams", strings.Join(out, "/"))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func classifyError(err error) string {
	switch {
	case err == nil:
		return "unknown"
	case errors.Is(err, context.Canceled):
		return "context_canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline"
	}
	ce := &websocket.CloseError{}
	if errors.As(err, &ce) {
		return fmt.Sprintf("close_%d", ce.Code)
	}
	if errors.Is(err, websocket.ErrReadLimit) {
		return "read_limit"
	}
	return "io_error"
}

// NextBackoff returns the next backoff duration with exponential growth and cap.
// Exposed for testing.
func NextBackoff(current, max time.Duration) time.Duration {
	next := current * 2
	if next > max {
		return max
	}
	return next
}
