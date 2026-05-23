package polymarket

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/shopspring/decimal"

	"github.com/adonmuhammaddd/poly-don-bot/internal/observability"
	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
)

type Config struct {
	WSURL             string
	RESTBaseURL       string
	SlugPrefix        string
	RequestsPerMinute int
	InitialBackoff    time.Duration
	MaxBackoff        time.Duration
	PingInterval      time.Duration
	ReadTimeout       time.Duration
}

type Storage interface {
	InsertPolymarketBook(ctx context.Context, arg postgres.InsertPolymarketBookParams) (int64, error)
}

type MarketFinder interface {
	FindCurrentMarket(ctx context.Context, slugPrefix string) (*Market, error)
}

type Dialer interface {
	DialContext(ctx context.Context, urlStr string, requestHeader http.Header) (*websocket.Conn, *http.Response, error)
}

type Client struct {
	cfg     Config
	rest    MarketFinder
	logger  *slog.Logger
	metrics *observability.Metrics
	storage Storage
	dialer  Dialer
}

func NewClient(cfg Config, logger *slog.Logger, metrics *observability.Metrics, storage Storage) *Client {
	return &Client{
		cfg:     cfg,
		rest:    NewRESTClient(cfg.RESTBaseURL, cfg.RequestsPerMinute, logger),
		logger:  logger.With(slog.String("component", "polymarket_feed")),
		metrics: metrics,
		storage: storage,
		dialer:  websocket.DefaultDialer,
	}
}

func (c *Client) WithDialer(d Dialer) *Client {
	c.dialer = d
	return c
}

func (c *Client) WithMarketFinder(m MarketFinder) *Client {
	c.rest = m
	return c
}

// Run discovers the current active market and streams its book updates. On
// disconnect or market expiry, it re-discovers and reconnects with backoff.
func (c *Client) Run(ctx context.Context) error {
	backoff := c.cfg.InitialBackoff
	for ctx.Err() == nil {
		market, err := c.rest.FindCurrentMarket(ctx, c.cfg.SlugPrefix)
		if err != nil {
			c.metrics.PolymarketRESTErrors.WithLabelValues("discovery").Inc()
			c.logger.Warn("market discovery failed", slog.Any("error", err))
			c.metrics.PolymarketActiveMarkets.Set(0)
		} else if market == nil {
			c.logger.Warn("no active market found for prefix", slog.String("prefix", c.cfg.SlugPrefix))
			c.metrics.PolymarketActiveMarkets.Set(0)
		} else {
			c.metrics.PolymarketActiveMarkets.Set(1)
			c.logger.Info("subscribing to market",
				slog.String("slug", market.Slug),
				slog.String("question", market.Question),
				slog.Time("end_date", market.EndDate),
			)

			// Run WS until disconnect OR market expiry (give 30s grace).
			grace := 30 * time.Second
			wsCtx, cancel := context.WithDeadline(ctx, market.EndDate.Add(grace))
			disconnectAt, runErr := c.runWS(wsCtx, market)
			cancel()

			if runErr != nil && ctx.Err() == nil {
				c.logger.Warn("ws session ended", slog.Any("error", runErr))
			}
			c.metrics.PolymarketReconnectSeconds.Observe(time.Since(disconnectAt).Seconds())
		}

		if ctx.Err() != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		backoff = nextBackoff(backoff, c.cfg.MaxBackoff)
	}
	return nil
}

func (c *Client) runWS(ctx context.Context, market *Market) (time.Time, error) {
	conn, resp, err := c.dialer.DialContext(ctx, c.cfg.WSURL, nil)
	if resp != nil {
		_ = resp.Body.Close()
	}
	if err != nil {
		c.metrics.PolymarketDisconnects.WithLabelValues("dial_failed").Inc()
		return time.Now(), fmt.Errorf("dial: %w", err)
	}
	defer func() { _ = conn.Close() }()

	sub := subscribeMessage{
		AssetsIDs:            []string{market.YesTokenID, market.NoTokenID},
		Type:                 "market",
		CustomFeatureEnabled: true,
	}
	if err := conn.WriteJSON(sub); err != nil {
		c.metrics.PolymarketDisconnects.WithLabelValues("subscribe_failed").Inc()
		return time.Now(), fmt.Errorf("subscribe: %w", err)
	}
	c.logger.Info("connected and subscribed")

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		<-ctx.Done()
		_ = conn.Close()
	}()

	// Polymarket expects a plain text "PING" every 10s.
	go func() {
		t := time.NewTicker(c.cfg.PingInterval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				if err := conn.WriteMessage(websocket.TextMessage, []byte("PING")); err != nil {
					return
				}
			}
		}
	}()

	_ = conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))
	for {
		_, raw, err := conn.ReadMessage()
		if err != nil {
			c.metrics.PolymarketDisconnects.WithLabelValues(classifyError(err)).Inc()
			return time.Now(), fmt.Errorf("read: %w", err)
		}
		_ = conn.SetReadDeadline(time.Now().Add(c.cfg.ReadTimeout))

		if err := c.handleMessage(ctx, market, raw); err != nil {
			c.logger.Warn("handle message failed", slog.Any("error", err))
		}
	}
}

func (c *Client) handleMessage(ctx context.Context, market *Market, raw []byte) error {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return nil
	}
	// Server PONG replies arrive as plain text, not JSON.
	if bytes.Equal(raw, []byte("PONG")) {
		return nil
	}

	// Server may batch events as a JSON array, or send a single object.
	if raw[0] == '[' {
		var events []json.RawMessage
		if err := json.Unmarshal(raw, &events); err != nil {
			return fmt.Errorf("unmarshal array: %w", err)
		}
		for _, ev := range events {
			if err := c.dispatchEvent(ctx, market, ev); err != nil {
				c.logger.Warn("dispatch failed", slog.Any("error", err))
			}
		}
		return nil
	}
	return c.dispatchEvent(ctx, market, raw)
}

func (c *Client) dispatchEvent(ctx context.Context, market *Market, raw []byte) error {
	var env eventEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("unmarshal envelope: %w", err)
	}
	switch env.EventType {
	case "book":
		return c.handleBookEvent(ctx, market, raw)
	case "price_change":
		return c.handlePriceChangeEvent(ctx, market, raw)
	default:
		return nil
	}
}

func (c *Client) handleBookEvent(ctx context.Context, market *Market, raw []byte) error {
	var ev BookEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return fmt.Errorf("unmarshal book: %w", err)
	}

	bid, hasBid := ev.BestBid()
	ask, hasAsk := ev.BestAsk()
	if !hasBid && !hasAsk {
		return nil
	}

	side := sideForAsset(ev.AssetID, market)
	if side == "" {
		return nil
	}

	params := buildInsertParams(market, raw, side, bid, ask, hasBid, hasAsk)
	if _, err := c.storage.InsertPolymarketBook(ctx, params); err != nil {
		return fmt.Errorf("insert book: %w", err)
	}
	c.metrics.PolymarketBookUpdates.WithLabelValues(side, "book").Inc()
	return nil
}

func (c *Client) handlePriceChangeEvent(ctx context.Context, market *Market, raw []byte) error {
	var ev PriceChangeEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		return fmt.Errorf("unmarshal price_change: %w", err)
	}

	for _, pc := range ev.PriceChanges {
		side := sideForAsset(pc.AssetID, market)
		if side == "" {
			continue
		}

		var (
			bid, ask         decimal.Decimal
			hasBid, hasAsk   bool
			parsedBid, _     = decimal.NewFromString(pc.BestBid)
			parsedAsk, errAk = decimal.NewFromString(pc.BestAsk)
		)
		if pc.BestBid != "" && parsedBid.GreaterThan(decimal.Zero) {
			bid, hasBid = parsedBid, true
		}
		if pc.BestAsk != "" && errAk == nil && parsedAsk.GreaterThan(decimal.Zero) {
			ask, hasAsk = parsedAsk, true
		}
		if !hasBid && !hasAsk {
			continue
		}

		params := buildInsertParams(market, raw, side, bid, ask, hasBid, hasAsk)
		if _, err := c.storage.InsertPolymarketBook(ctx, params); err != nil {
			return fmt.Errorf("insert price_change: %w", err)
		}
		c.metrics.PolymarketBookUpdates.WithLabelValues(side, "price_change").Inc()
	}
	return nil
}

func sideForAsset(assetID string, market *Market) string {
	switch assetID {
	case market.YesTokenID:
		return "yes"
	case market.NoTokenID:
		return "no"
	default:
		return ""
	}
}

func buildInsertParams(market *Market, raw []byte, side string, bid, ask decimal.Decimal, hasBid, hasAsk bool) postgres.InsertPolymarketBookParams {
	p := postgres.InsertPolymarketBookParams{
		MarketID:   market.ConditionID,
		Question:   market.Question,
		RawPayload: raw,
	}
	if side == "yes" {
		if hasBid {
			p.YesBid = decimal.NewNullDecimal(bid)
		}
		if hasAsk {
			p.YesAsk = decimal.NewNullDecimal(ask)
		}
	} else {
		if hasBid {
			p.NoBid = decimal.NewNullDecimal(bid)
		}
		if hasAsk {
			p.NoAsk = decimal.NewNullDecimal(ask)
		}
	}
	return p
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
	return "io_error"
}

func nextBackoff(cur, max time.Duration) time.Duration {
	next := cur * 2
	if next > max {
		return max
	}
	return next
}

// NextBackoff is exposed for tests.
func NextBackoff(cur, max time.Duration) time.Duration {
	return nextBackoff(cur, max)
}
