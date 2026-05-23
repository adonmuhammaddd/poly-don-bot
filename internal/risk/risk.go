package risk

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"

	"github.com/adonmuhammaddd/poly-don-bot/internal/strategy"
)

// minPositionSizeUSD is the smallest paper/live order we will route. Anything
// below this is rejected so we don't waste fees on dust trades.
var minPositionSizeUSD = decimal.NewFromInt(1)

// Risk enforces the limits in Config. Decide() is goroutine-safe.
// Circuit-breaker state lives here; daily counters come from AccountState.
type Risk struct {
	cfg Config

	mu               sync.Mutex
	breakerStartedAt time.Time
}

func NewRisk(cfg Config) *Risk {
	return &Risk{cfg: applyDefaults(cfg)}
}

func applyDefaults(cfg Config) Config {
	if cfg.MaxPositionSizeUSD.IsZero() {
		cfg.MaxPositionSizeUSD = decimal.NewFromInt(50)
	}
	if cfg.MaxPositionPctBalance.IsZero() {
		cfg.MaxPositionPctBalance = decimal.NewFromFloat(0.05)
	}
	if cfg.MaxTradesPerDay <= 0 {
		cfg.MaxTradesPerDay = 30
	}
	if cfg.MaxDailyLossUSD.IsZero() {
		cfg.MaxDailyLossUSD = decimal.NewFromInt(30)
	}
	if cfg.MaxDailyLossPct.IsZero() {
		cfg.MaxDailyLossPct = decimal.NewFromFloat(0.15)
	}
	if cfg.MaxConsecutiveLosses <= 0 {
		cfg.MaxConsecutiveLosses = 5
	}
	if cfg.MaxSignalAge <= 0 {
		cfg.MaxSignalAge = 500 * time.Millisecond
	}
	if cfg.MinBalanceUSD.IsZero() {
		cfg.MinBalanceUSD = decimal.NewFromInt(50)
	}
	if cfg.CircuitBreakerPause <= 0 {
		cfg.CircuitBreakerPause = time.Hour
	}
	return cfg
}

// Decide is the gate every signal must pass before becoming a trade. The
// order of checks is deliberate — cheaper checks first, and we return on
// the first rejection so the caller can attribute one clear reason.
func (r *Risk) Decide(sig *strategy.Signal, acct AccountState) Decision {
	if sig == nil {
		return Decision{Reason: ReasonSignalStale}
	}

	age := acct.Now.Sub(sig.DetectedAt)
	if age > r.cfg.MaxSignalAge {
		return Decision{Reason: ReasonSignalStale}
	}

	if acct.BalanceUSD.LessThan(r.cfg.MinBalanceUSD) {
		return Decision{Reason: ReasonLowBalance}
	}

	if acct.TodayPnLUSD.Neg().GreaterThanOrEqual(r.cfg.MaxDailyLossUSD) {
		return Decision{Reason: ReasonDailyLossUSD}
	}

	if !acct.StartOfDayBalance.IsZero() {
		lossPct := acct.TodayPnLUSD.Neg().Div(acct.StartOfDayBalance)
		if lossPct.GreaterThanOrEqual(r.cfg.MaxDailyLossPct) {
			return Decision{Reason: ReasonDailyLossPct}
		}
	}

	if acct.TodayTradesCount >= r.cfg.MaxTradesPerDay {
		return Decision{Reason: ReasonDailyTrades}
	}

	if r.breakerActive(acct.Now, acct.ConsecutiveLosses) {
		return Decision{Reason: ReasonBreakerActive}
	}

	size := r.sizePosition(acct.BalanceUSD)
	if size.LessThan(minPositionSizeUSD) {
		return Decision{Reason: ReasonSizeTooSmall}
	}

	return Decision{
		Approved:        true,
		Reason:          ReasonApproved,
		PositionSizeUSD: size,
	}
}

// sizePosition returns min(MaxPositionSizeUSD, balance * MaxPositionPctBalance).
func (r *Risk) sizePosition(balance decimal.Decimal) decimal.Decimal {
	pctSize := balance.Mul(r.cfg.MaxPositionPctBalance)
	if pctSize.LessThan(r.cfg.MaxPositionSizeUSD) {
		return pctSize
	}
	return r.cfg.MaxPositionSizeUSD
}

// breakerActive returns true when the loss streak threshold has been crossed
// AND the pause window hasn't elapsed yet.
func (r *Risk) breakerActive(now time.Time, consecutiveLosses int) bool {
	if consecutiveLosses < r.cfg.MaxConsecutiveLosses {
		return false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.breakerStartedAt.IsZero() {
		r.breakerStartedAt = now
	}
	return now.Sub(r.breakerStartedAt) < r.cfg.CircuitBreakerPause
}

// ResetBreaker clears any active circuit breaker. The caller invokes this
// when the loss streak resets (e.g. after a winning trade).
func (r *Risk) ResetBreaker() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.breakerStartedAt = time.Time{}
}

// SignalToDecide is a convenience used by tests and callers that already
// have a *strategy.Signal handy.
var _ = strategy.Signal{}
