package signals

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/adonmuhammaddd/poly-don-bot/internal/observability"
	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
	"github.com/adonmuhammaddd/poly-don-bot/internal/strategy"
)

func TestOnSignal_PersistsWithPolymarketContext(t *testing.T) {
	now := time.Now().UTC()
	sig := &strategy.Signal{
		Symbol:     "btcusdt",
		Direction:  strategy.DirectionUp,
		Magnitude:  decimal.NewFromFloat(0.0025),
		Confidence: decimal.NewFromFloat(0.75),
		DetectedAt: now,
		WindowMs:   1500,
		Context: strategy.Context{
			BinancePrice:     decimal.NewFromInt(65000),
			WindowStartPrice: decimal.NewFromFloat(64837.5),
		},
	}
	store := &fakeStorage{
		market: postgres.GetLatestActiveMarketRow{
			MarketID: "0xabc",
			Question: "Bitcoin Up or Down - test",
			LastSeen: pgtype.Timestamptz{Time: now, Valid: true},
		},
		yes: postgres.GetLatestYesQuoteRow{
			YesBid: decimal.NewNullDecimal(decimal.NewFromFloat(0.49)),
			YesAsk: decimal.NewNullDecimal(decimal.NewFromFloat(0.51)),
		},
	}
	p := newTestPersister(store)
	p.OnSignal(sig)

	if len(store.inserts) != 1 {
		t.Fatalf("got %d inserts want 1", len(store.inserts))
	}
	ins := store.inserts[0]
	if ins.Direction != "up" {
		t.Errorf("direction=%s", ins.Direction)
	}
	if ins.WindowMs != 1500 {
		t.Errorf("WindowMs=%d", ins.WindowMs)
	}
	if !ins.Confidence.Equal(decimal.NewFromFloat(0.75)) {
		t.Errorf("Confidence=%s", ins.Confidence)
	}
	if ins.ActionTaken.String != "skipped" || ins.ActionReason.String != "phase_2_observation" {
		t.Errorf("action=%+v reason=%+v", ins.ActionTaken, ins.ActionReason)
	}

	var ctx signalContext
	if err := json.Unmarshal(ins.Context, &ctx); err != nil {
		t.Fatalf("unmarshal context: %v", err)
	}
	if ctx.Polymarket == nil {
		t.Fatal("expected polymarket snapshot")
	}
	if ctx.Polymarket.MarketID != "0xabc" {
		t.Errorf("MarketID=%s", ctx.Polymarket.MarketID)
	}
	if ctx.Polymarket.YesMid == nil || *ctx.Polymarket.YesMid != "0.5000" {
		t.Errorf("YesMid=%v", ctx.Polymarket.YesMid)
	}
}

func TestOnSignal_OmitsPolymarketWhenNoMarket(t *testing.T) {
	store := &fakeStorage{marketErr: pgx.ErrNoRows}
	p := newTestPersister(store)
	p.OnSignal(validSignal())

	if len(store.inserts) != 1 {
		t.Fatalf("got %d inserts want 1", len(store.inserts))
	}
	var ctx signalContext
	if err := json.Unmarshal(store.inserts[0].Context, &ctx); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ctx.Polymarket != nil {
		t.Errorf("Polymarket should be nil: %+v", ctx.Polymarket)
	}
}

func TestOnSignal_IncludesPartialQuoteWhenAskMissing(t *testing.T) {
	store := &fakeStorage{
		market: postgres.GetLatestActiveMarketRow{MarketID: "0xabc", Question: "q"},
		yes: postgres.GetLatestYesQuoteRow{
			YesBid: decimal.NewNullDecimal(decimal.NewFromFloat(0.49)),
		},
	}
	p := newTestPersister(store)
	p.OnSignal(validSignal())

	var ctx signalContext
	_ = json.Unmarshal(store.inserts[0].Context, &ctx)
	if ctx.Polymarket.YesBid == nil || *ctx.Polymarket.YesBid != "0.49" {
		t.Errorf("YesBid=%v", ctx.Polymarket.YesBid)
	}
	if ctx.Polymarket.YesMid != nil {
		t.Errorf("YesMid should be nil when only bid present: %v", ctx.Polymarket.YesMid)
	}
}

func TestOnSignal_InsertErrorDoesNotPanic(t *testing.T) {
	store := &fakeStorage{insertErr: errors.New("db down")}
	p := newTestPersister(store)
	p.OnSignal(validSignal())
	if len(store.inserts) != 0 {
		t.Errorf("expected insert to fail, got %d", len(store.inserts))
	}
}

func validSignal() *strategy.Signal {
	return &strategy.Signal{
		Symbol:     "btcusdt",
		Direction:  strategy.DirectionUp,
		Magnitude:  decimal.NewFromFloat(0.0025),
		Confidence: decimal.NewFromFloat(0.75),
		DetectedAt: time.Now().UTC(),
		WindowMs:   1000,
	}
}

func newTestPersister(store Storage) *Persister {
	return NewPersister(store, observability.NewMetrics(), testLogger())
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeStorage struct {
	mu sync.Mutex

	inserts   []postgres.InsertSignalParams
	insertErr error

	market    postgres.GetLatestActiveMarketRow
	marketErr error

	yes    postgres.GetLatestYesQuoteRow
	yesErr error
}

func (f *fakeStorage) InsertSignal(_ context.Context, arg postgres.InsertSignalParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return 0, f.insertErr
	}
	f.inserts = append(f.inserts, arg)
	return int64(len(f.inserts)), nil
}

func (f *fakeStorage) GetLatestActiveMarket(_ context.Context, _ pgtype.Timestamptz) (postgres.GetLatestActiveMarketRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.market, f.marketErr
}

func (f *fakeStorage) GetLatestYesQuote(_ context.Context, _ string) (postgres.GetLatestYesQuoteRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.yes, f.yesErr
}
