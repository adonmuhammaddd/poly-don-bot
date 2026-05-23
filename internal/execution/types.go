package execution

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/adonmuhammaddd/poly-don-bot/internal/risk"
	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
	"github.com/adonmuhammaddd/poly-don-bot/internal/strategy"
)

const (
	SideYES = "YES"
	SideNO  = "NO"

	ModePaper = "paper"
	ModeLive  = "live"

	StatusOpen   = "open"
	StatusClosed = "closed"
	StatusFailed = "failed"
)

// Skip reasons surfaced to metrics. Risk-rejection reasons come from the
// risk package; these are execution-layer skips (no market, missing quote).
const (
	SkipReasonNoMarket     = "no_market"
	SkipReasonNoQuote      = "no_quote"
	SkipReasonInsertFail   = "insert_failed"
	SkipReasonZeroAsk      = "zero_ask"
	SkipReasonPriceTooHigh = "price_too_high"
)

// PaperConfig configures the paper executor. Defaults align with the
// $100 paper balance choice made in the Phase 3 plan.
type PaperConfig struct {
	StartingBalanceUSD decimal.Decimal
	SlippageTick       decimal.Decimal
	MaxFillPrice       decimal.Decimal
	MarketLookbackTime time.Duration
}

// RiskGate is the slice of risk.Risk that executor needs.
type RiskGate interface {
	Decide(sig *strategy.Signal, acct risk.AccountState) risk.Decision
}

// Storage is the slice of *postgres.Queries that the paper executor uses.
type Storage interface {
	GetLatestActiveMarket(ctx context.Context, since pgtype.Timestamptz) (postgres.GetLatestActiveMarketRow, error)
	GetLatestYesQuote(ctx context.Context, marketID string) (postgres.GetLatestYesQuoteRow, error)
	GetLatestNoQuote(ctx context.Context, marketID string) (postgres.GetLatestNoQuoteRow, error)
	InsertTrade(ctx context.Context, arg postgres.InsertTradeParams) (int64, error)
	ListOpenPaperTradeSizes(ctx context.Context) ([]decimal.Decimal, error)
	CountTodayPaperTrades(ctx context.Context, openedAt pgtype.Timestamptz) (int64, error)
}
