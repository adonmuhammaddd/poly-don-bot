package execution

import (
	"context"
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
	"github.com/adonmuhammaddd/poly-don-bot/internal/risk"
	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
	"github.com/adonmuhammaddd/poly-don-bot/internal/strategy"
)

func freshSignal(now time.Time, dir strategy.Direction) *strategy.Signal {
	return &strategy.Signal{
		Symbol:     "btcusdt",
		Direction:  dir,
		Magnitude:  decimal.NewFromFloat(0.0025),
		Confidence: decimal.NewFromFloat(0.75),
		DetectedAt: now,
		WindowMs:   1000,
	}
}

func TestPaperExecutor_OpensYESTradeOnUpSignal(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.market = postgres.GetLatestActiveMarketRow{
		MarketID: "0xabc",
		Question: "Bitcoin Up or Down - test",
		LastSeen: pgtype.Timestamptz{Time: now, Valid: true},
	}
	store.yes = postgres.GetLatestYesQuoteRow{
		YesBid: decimal.NewNullDecimal(decimal.NewFromFloat(0.49)),
		YesAsk: decimal.NewNullDecimal(decimal.NewFromFloat(0.51)),
	}

	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionUp))

	if len(store.trades) != 1 {
		t.Fatalf("got %d trades want 1", len(store.trades))
	}
	tr := store.trades[0]
	if tr.Mode != ModePaper {
		t.Errorf("Mode=%s", tr.Mode)
	}
	if tr.Side != SideYES {
		t.Errorf("Side=%s want YES", tr.Side)
	}
	expected := decimal.NewFromFloat(0.52) // 0.51 ask + 0.01 slippage
	if !tr.EntryPrice.Equal(expected) {
		t.Errorf("EntryPrice=%s want %s", tr.EntryPrice, expected)
	}
	if !tr.SizeUsd.Equal(decimal.NewFromInt(5)) { // 5% of 100
		t.Errorf("SizeUsd=%s want 5", tr.SizeUsd)
	}
	if tr.Status != StatusOpen {
		t.Errorf("Status=%s", tr.Status)
	}
}

func TestPaperExecutor_OpensNOTradeOnDownSignal(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.market = postgres.GetLatestActiveMarketRow{MarketID: "0xabc"}
	store.no = postgres.GetLatestNoQuoteRow{
		NoBid: decimal.NewNullDecimal(decimal.NewFromFloat(0.47)),
		NoAsk: decimal.NewNullDecimal(decimal.NewFromFloat(0.49)),
	}

	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionDown))

	if len(store.trades) != 1 {
		t.Fatalf("got %d trades", len(store.trades))
	}
	if store.trades[0].Side != SideNO {
		t.Errorf("Side=%s want NO", store.trades[0].Side)
	}
	expected := decimal.NewFromFloat(0.50)
	if !store.trades[0].EntryPrice.Equal(expected) {
		t.Errorf("EntryPrice=%s want %s", store.trades[0].EntryPrice, expected)
	}
}

func TestPaperExecutor_RiskRejectSkipsTrade(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	stale := freshSignal(now.Add(-time.Second), strategy.DirectionUp) // 1s old > 500ms
	exec.OnSignal(stale)

	if len(store.trades) != 0 {
		t.Errorf("expected no trades on risk reject")
	}
}

func TestPaperExecutor_NoActiveMarketSkips(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.marketErr = pgx.ErrNoRows
	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionUp))

	if len(store.trades) != 0 {
		t.Errorf("expected no trades when no market: %+v", store.trades)
	}
}

func TestPaperExecutor_NoQuoteSkips(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.market = postgres.GetLatestActiveMarketRow{MarketID: "0xabc"}
	store.yesErr = pgx.ErrNoRows
	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionUp))

	if len(store.trades) != 0 {
		t.Errorf("expected no trades when no quote")
	}
}

func TestPaperExecutor_AskMissingSkips(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.market = postgres.GetLatestActiveMarketRow{MarketID: "0xabc"}
	store.yes = postgres.GetLatestYesQuoteRow{
		YesBid: decimal.NewNullDecimal(decimal.NewFromFloat(0.49)),
		// YesAsk left invalid
	}
	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionUp))

	if len(store.trades) != 0 {
		t.Errorf("expected no trades when ask missing")
	}
}

func TestPaperExecutor_AskTooHighRejects(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.market = postgres.GetLatestActiveMarketRow{MarketID: "0xabc"}
	store.yes = postgres.GetLatestYesQuoteRow{
		YesBid: decimal.NewNullDecimal(decimal.NewFromFloat(0.98)),
		YesAsk: decimal.NewNullDecimal(decimal.NewFromFloat(0.99)),
	}
	// fill = 0.99 + 0.01 = 1.00 > MaxFillPrice 0.99 → reject
	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionUp))

	if len(store.trades) != 0 {
		t.Errorf("expected reject when fill > MaxFillPrice")
	}
}

func TestPaperExecutor_NoAskMissingSkips(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.market = postgres.GetLatestActiveMarketRow{MarketID: "0xabc"}
	store.no = postgres.GetLatestNoQuoteRow{
		NoBid: decimal.NewNullDecimal(decimal.NewFromFloat(0.47)),
	}
	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionDown))

	if len(store.trades) != 0 {
		t.Errorf("expected no trade when NO ask missing")
	}
}

func TestPaperExecutor_AccountStateErrorSkips(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.openSizesErr = errors.New("db down")
	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionUp))

	if len(store.trades) != 0 {
		t.Errorf("expected no trade on account state error")
	}
}

func TestPaperExecutor_CountTodayErrorSkips(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.countTodayErr = errors.New("db down")
	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionUp))

	if len(store.trades) != 0 {
		t.Errorf("expected no trade on count error")
	}
}

func TestPaperExecutor_BalanceShrinksAsTradesOpen(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.market = postgres.GetLatestActiveMarketRow{MarketID: "0xabc"}
	store.yes = postgres.GetLatestYesQuoteRow{
		YesBid: decimal.NewNullDecimal(decimal.NewFromFloat(0.49)),
		YesAsk: decimal.NewNullDecimal(decimal.NewFromFloat(0.51)),
	}
	// Cap at 60 size_usd already committed: balance = 100 - 60 = 40 < $50 min.
	store.openSizes = []decimal.Decimal{decimal.NewFromInt(60)}

	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionUp))

	if len(store.trades) != 0 {
		t.Errorf("expected risk reject when committed reduces balance below min")
	}
}

func TestPaperExecutor_InsertErrorReportsSkip(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.market = postgres.GetLatestActiveMarketRow{MarketID: "0xabc"}
	store.yes = postgres.GetLatestYesQuoteRow{
		YesBid: decimal.NewNullDecimal(decimal.NewFromFloat(0.49)),
		YesAsk: decimal.NewNullDecimal(decimal.NewFromFloat(0.51)),
	}
	store.insertErr = errors.New("insert fail")

	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionUp))

	if len(store.trades) != 0 {
		t.Errorf("expected trades empty on insert error: %+v", store.trades)
	}
}

func TestPaperExecutor_MarketLookupOtherErrorSkips(t *testing.T) {
	now := time.Now().UTC()
	store := newFakeStorage()
	store.marketErr = errors.New("conn reset")
	exec := newTestExecutor(store, risk.NewRisk(risk.Config{}))
	exec.OnSignal(freshSignal(now, strategy.DirectionUp))

	if len(store.trades) != 0 {
		t.Errorf("expected no trades on market lookup error")
	}
}

func newTestExecutor(store Storage, r RiskGate) *PaperExecutor {
	return NewPaperExecutor(PaperConfig{}, store, r, observability.NewMetrics(), testLogger())
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

type fakeStorage struct {
	mu sync.Mutex

	market    postgres.GetLatestActiveMarketRow
	marketErr error

	yes    postgres.GetLatestYesQuoteRow
	yesErr error

	no    postgres.GetLatestNoQuoteRow
	noErr error

	trades    []postgres.InsertTradeParams
	insertErr error

	openSizes    []decimal.Decimal
	openSizesErr error

	countToday    int64
	countTodayErr error
}

func newFakeStorage() *fakeStorage {
	return &fakeStorage{}
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

func (f *fakeStorage) GetLatestNoQuote(_ context.Context, _ string) (postgres.GetLatestNoQuoteRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.no, f.noErr
}

func (f *fakeStorage) InsertTrade(_ context.Context, arg postgres.InsertTradeParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return 0, f.insertErr
	}
	f.trades = append(f.trades, arg)
	return int64(len(f.trades)), nil
}

func (f *fakeStorage) ListOpenPaperTradeSizes(_ context.Context) ([]decimal.Decimal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.openSizes, f.openSizesErr
}

func (f *fakeStorage) CountTodayPaperTrades(_ context.Context, _ pgtype.Timestamptz) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.countToday, f.countTodayErr
}
