package risk

import (
	"time"

	"github.com/shopspring/decimal"
)

// Config lists the risk limits. Defaults mirror HANDOFF Section 6 and
// represent the "Phase 4 live" caps; Phase 3 paper trading uses the same
// caps so the simulation reflects how live execution would gate it.
type Config struct {
	MaxPositionSizeUSD    decimal.Decimal
	MaxPositionPctBalance decimal.Decimal
	MaxTradesPerDay       int
	MaxDailyLossUSD       decimal.Decimal
	MaxDailyLossPct       decimal.Decimal
	MaxConsecutiveLosses  int
	MaxSignalAge          time.Duration
	MinBalanceUSD         decimal.Decimal
	CircuitBreakerPause   time.Duration
}

// AccountState is the snapshot the caller passes to Decide(). The risk
// package does not own this state — the caller (paper executor / PnL
// aggregator) computes it from trade history.
type AccountState struct {
	BalanceUSD        decimal.Decimal
	StartOfDayBalance decimal.Decimal
	TodayPnLUSD       decimal.Decimal
	TodayTradesCount  int
	ConsecutiveLosses int
	Now               time.Time
}

// Reason codes for rejected decisions. Stable strings — surfaced in
// trades.action_reason and Prometheus labels.
const (
	ReasonApproved      = "approved"
	ReasonSignalStale   = "signal_stale"
	ReasonLowBalance    = "low_balance"
	ReasonDailyLossUSD  = "daily_loss_usd"
	ReasonDailyLossPct  = "daily_loss_pct"
	ReasonDailyTrades   = "daily_trades_cap"
	ReasonBreakerActive = "circuit_breaker"
	ReasonSizeTooSmall  = "size_too_small"
)

// Decision is the output of Decide(). When Approved=false, PositionSizeUSD
// is zero and Reason explains the rejection.
type Decision struct {
	Approved        bool
	Reason          string
	PositionSizeUSD decimal.Decimal
}
