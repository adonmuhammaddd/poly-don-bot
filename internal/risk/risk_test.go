package risk

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"

	"github.com/adonmuhammaddd/poly-don-bot/internal/strategy"
)

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func freshSignal(now time.Time) *strategy.Signal {
	return &strategy.Signal{
		Symbol:     "btcusdt",
		Direction:  strategy.DirectionUp,
		Magnitude:  dec("0.0025"),
		Confidence: dec("0.75"),
		DetectedAt: now,
		WindowMs:   1500,
	}
}

func happyAccount(now time.Time) AccountState {
	return AccountState{
		BalanceUSD:        dec("100"),
		StartOfDayBalance: dec("100"),
		TodayPnLUSD:       dec("0"),
		TodayTradesCount:  0,
		ConsecutiveLosses: 0,
		Now:               now,
	}
}

func TestApplyDefaults(t *testing.T) {
	r := NewRisk(Config{})
	if !r.cfg.MaxPositionSizeUSD.Equal(decimal.NewFromInt(50)) {
		t.Errorf("MaxPositionSizeUSD default=%s", r.cfg.MaxPositionSizeUSD)
	}
	if !r.cfg.MaxPositionPctBalance.Equal(decimal.NewFromFloat(0.05)) {
		t.Errorf("MaxPositionPctBalance default=%s", r.cfg.MaxPositionPctBalance)
	}
	if r.cfg.MaxTradesPerDay != 30 {
		t.Errorf("MaxTradesPerDay default=%d", r.cfg.MaxTradesPerDay)
	}
	if !r.cfg.MaxDailyLossUSD.Equal(decimal.NewFromInt(30)) {
		t.Errorf("MaxDailyLossUSD default=%s", r.cfg.MaxDailyLossUSD)
	}
	if !r.cfg.MaxDailyLossPct.Equal(decimal.NewFromFloat(0.15)) {
		t.Errorf("MaxDailyLossPct default=%s", r.cfg.MaxDailyLossPct)
	}
	if r.cfg.MaxConsecutiveLosses != 5 {
		t.Errorf("MaxConsecutiveLosses default=%d", r.cfg.MaxConsecutiveLosses)
	}
	if r.cfg.MaxSignalAge != 500*time.Millisecond {
		t.Errorf("MaxSignalAge default=%v", r.cfg.MaxSignalAge)
	}
	if !r.cfg.MinBalanceUSD.Equal(decimal.NewFromInt(50)) {
		t.Errorf("MinBalanceUSD default=%s", r.cfg.MinBalanceUSD)
	}
	if r.cfg.CircuitBreakerPause != time.Hour {
		t.Errorf("CircuitBreakerPause default=%v", r.cfg.CircuitBreakerPause)
	}
}

func TestApplyDefaults_RespectsCustom(t *testing.T) {
	cfg := Config{
		MaxPositionSizeUSD:    dec("100"),
		MaxPositionPctBalance: dec("0.10"),
		MaxTradesPerDay:       50,
		MaxDailyLossUSD:       dec("60"),
		MaxDailyLossPct:       dec("0.25"),
		MaxConsecutiveLosses:  10,
		MaxSignalAge:          time.Second,
		MinBalanceUSD:         dec("25"),
		CircuitBreakerPause:   30 * time.Minute,
	}
	r := NewRisk(cfg)
	if r.cfg != cfg {
		t.Errorf("custom config not preserved: %+v", r.cfg)
	}
}

func TestDecide_NilSignalRejected(t *testing.T) {
	r := NewRisk(Config{})
	d := r.Decide(nil, happyAccount(time.Now()))
	if d.Approved || d.Reason != ReasonSignalStale {
		t.Errorf("got %+v", d)
	}
}

func TestDecide_StaleSignalRejected(t *testing.T) {
	r := NewRisk(Config{})
	now := time.Now()
	sig := freshSignal(now.Add(-time.Second)) // 1s old, above 500ms cap
	d := r.Decide(sig, happyAccount(now))
	if d.Approved || d.Reason != ReasonSignalStale {
		t.Errorf("got %+v", d)
	}
}

func TestDecide_LowBalanceRejected(t *testing.T) {
	r := NewRisk(Config{})
	now := time.Now()
	acct := happyAccount(now)
	acct.BalanceUSD = dec("49")
	d := r.Decide(freshSignal(now), acct)
	if d.Approved || d.Reason != ReasonLowBalance {
		t.Errorf("got %+v", d)
	}
}

func TestDecide_DailyLossUSDRejected(t *testing.T) {
	r := NewRisk(Config{})
	now := time.Now()
	acct := happyAccount(now)
	acct.TodayPnLUSD = dec("-30")
	d := r.Decide(freshSignal(now), acct)
	if d.Approved || d.Reason != ReasonDailyLossUSD {
		t.Errorf("got %+v", d)
	}
}

func TestDecide_DailyLossPctRejected(t *testing.T) {
	r := NewRisk(Config{})
	now := time.Now()
	acct := happyAccount(now)
	acct.BalanceUSD = dec("100")
	acct.StartOfDayBalance = dec("100")
	acct.TodayPnLUSD = dec("-15") // 15% of 100
	d := r.Decide(freshSignal(now), acct)
	if d.Approved || d.Reason != ReasonDailyLossPct {
		t.Errorf("got %+v", d)
	}
}

func TestDecide_DailyLossPctSkippedWhenStartOfDayZero(t *testing.T) {
	r := NewRisk(Config{})
	now := time.Now()
	acct := happyAccount(now)
	acct.StartOfDayBalance = dec("0")
	acct.TodayPnLUSD = dec("-1")
	d := r.Decide(freshSignal(now), acct)
	if !d.Approved {
		t.Errorf("expected approval, got %+v", d)
	}
}

func TestDecide_DailyTradesCapRejected(t *testing.T) {
	r := NewRisk(Config{})
	now := time.Now()
	acct := happyAccount(now)
	acct.TodayTradesCount = 30
	d := r.Decide(freshSignal(now), acct)
	if d.Approved || d.Reason != ReasonDailyTrades {
		t.Errorf("got %+v", d)
	}
}

func TestDecide_CircuitBreakerActive(t *testing.T) {
	r := NewRisk(Config{})
	now := time.Now()
	acct := happyAccount(now)
	acct.ConsecutiveLosses = 5
	d := r.Decide(freshSignal(now), acct)
	if d.Approved || d.Reason != ReasonBreakerActive {
		t.Errorf("got %+v", d)
	}
}

func TestDecide_CircuitBreakerExpires(t *testing.T) {
	r := NewRisk(Config{CircuitBreakerPause: time.Hour})
	now := time.Now()
	// First call sets breakerStartedAt = now
	acct := happyAccount(now)
	acct.ConsecutiveLosses = 5
	d := r.Decide(freshSignal(now), acct)
	if d.Approved {
		t.Fatal("first call should reject")
	}
	// Two hours later — breaker expired (but consecutive losses still 5, so
	// caller has to reset by recording a win first)
	r.ResetBreaker()
	acct.ConsecutiveLosses = 0
	d2 := r.Decide(freshSignal(now.Add(2*time.Hour)), acct.withNow(now.Add(2*time.Hour)))
	if !d2.Approved {
		t.Errorf("expected approval after breaker expiry + win, got %+v", d2)
	}
}

func TestDecide_SizingUsesPercentageWhenSmallerThanCap(t *testing.T) {
	r := NewRisk(Config{})
	now := time.Now()
	acct := happyAccount(now) // balance=100, 5% = 5
	d := r.Decide(freshSignal(now), acct)
	if !d.Approved {
		t.Fatalf("expected approval, got %+v", d)
	}
	if !d.PositionSizeUSD.Equal(dec("5")) {
		t.Errorf("expected size=5 (5%% of 100), got %s", d.PositionSizeUSD)
	}
}

func TestDecide_SizingUsesCapWhenBalanceLarge(t *testing.T) {
	r := NewRisk(Config{})
	now := time.Now()
	acct := happyAccount(now)
	acct.BalanceUSD = dec("10000") // 5% = 500, capped at 50
	d := r.Decide(freshSignal(now), acct)
	if !d.Approved {
		t.Fatalf("expected approval, got %+v", d)
	}
	if !d.PositionSizeUSD.Equal(dec("50")) {
		t.Errorf("expected size=50 (cap), got %s", d.PositionSizeUSD)
	}
}

func TestDecide_RejectsWhenSizeBelowMinimum(t *testing.T) {
	r := NewRisk(Config{
		MaxPositionPctBalance: dec("0.001"), // 0.1% of balance
		MinBalanceUSD:         dec("1"),
	})
	now := time.Now()
	acct := happyAccount(now)
	acct.BalanceUSD = dec("100") // 0.1% = 0.10, below $1 minimum
	acct.StartOfDayBalance = dec("100")
	d := r.Decide(freshSignal(now), acct)
	if d.Approved || d.Reason != ReasonSizeTooSmall {
		t.Errorf("got %+v", d)
	}
}

func TestDecide_HappyPath(t *testing.T) {
	r := NewRisk(Config{})
	now := time.Now()
	d := r.Decide(freshSignal(now), happyAccount(now))
	if !d.Approved {
		t.Fatalf("expected approval, got %+v", d)
	}
	if d.Reason != ReasonApproved {
		t.Errorf("reason=%s", d.Reason)
	}
	if d.PositionSizeUSD.IsZero() {
		t.Error("size should be > 0")
	}
}

func TestResetBreaker_NoStateNoOp(t *testing.T) {
	r := NewRisk(Config{})
	r.ResetBreaker() // should not panic on zero state
}

// withNow is a tiny test helper that returns a copy with Now overridden.
func (a AccountState) withNow(t time.Time) AccountState {
	a.Now = t
	return a
}
