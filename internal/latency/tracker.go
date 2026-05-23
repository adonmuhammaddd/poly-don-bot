package latency

import (
	"sort"
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// Config controls move detection thresholds and window sizes.
type Config struct {
	// SignificantMoveThreshold is the relative price change that counts as a move.
	// 0.0005 = 0.05%.
	SignificantMoveThreshold decimal.Decimal
	// MaxPairAge drops pending moves that don't get paired in time.
	MaxPairAge time.Duration
	// WindowDuration is how long measurements stay in the stats window.
	WindowDuration time.Duration
}

// Measurement is a single Binance-move → Polymarket-reprice pair.
type Measurement struct {
	BinanceMoveAt     time.Time
	PolymarketReprice time.Time
	DeltaMs           int64
}

// Stats summarises measurements in the rolling window.
type Stats struct {
	Count        int   `json:"count"`
	AvgMs        int64 `json:"avgMs"`
	P50Ms        int64 `json:"p50Ms"`
	P95Ms        int64 `json:"p95Ms"`
	LastDeltaMs  int64 `json:"lastDeltaMs"`
	WindowSecs   int   `json:"windowSecs"`
	PendingMoves int   `json:"pendingMoves"`
}

// Tracker is goroutine-safe.
type Tracker struct {
	cfg Config
	mu  sync.Mutex

	referencePrice decimal.Decimal
	pendingMoves   []time.Time
	measurements   []Measurement
}

func NewTracker(cfg Config) *Tracker {
	if cfg.SignificantMoveThreshold.IsZero() {
		cfg.SignificantMoveThreshold = decimal.NewFromFloat(0.0005)
	}
	if cfg.MaxPairAge <= 0 {
		cfg.MaxPairAge = 30 * time.Second
	}
	if cfg.WindowDuration <= 0 {
		cfg.WindowDuration = 60 * time.Second
	}
	return &Tracker{cfg: cfg}
}

// OnTick records a Binance price observation. If the relative move from the
// last reference price exceeds the threshold, it becomes a pending move.
func (t *Tracker) OnTick(price decimal.Decimal, ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.referencePrice.IsZero() {
		t.referencePrice = price
		return
	}

	delta := price.Sub(t.referencePrice).Abs()
	threshold := t.referencePrice.Mul(t.cfg.SignificantMoveThreshold)
	if delta.GreaterThan(threshold) {
		t.pendingMoves = append(t.pendingMoves, ts)
		t.referencePrice = price
		t.dropStalePending(ts)
	}
}

// OnBookUpdate records a Polymarket book/price_change observation. The oldest
// pending Binance move is paired with this update.
func (t *Tracker) OnBookUpdate(ts time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.dropStalePending(ts)
	if len(t.pendingMoves) == 0 {
		return
	}

	move := t.pendingMoves[0]
	t.pendingMoves = t.pendingMoves[1:]
	if ts.Before(move) {
		return
	}

	t.measurements = append(t.measurements, Measurement{
		BinanceMoveAt:     move,
		PolymarketReprice: ts,
		DeltaMs:           ts.Sub(move).Milliseconds(),
	})
	t.trimWindow(ts)
}

func (t *Tracker) Stats() Stats {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.trimWindow(time.Now().UTC())

	out := Stats{
		WindowSecs:   int(t.cfg.WindowDuration.Seconds()),
		PendingMoves: len(t.pendingMoves),
	}
	if len(t.measurements) == 0 {
		return out
	}

	deltas := make([]int64, len(t.measurements))
	var sum int64
	for i, m := range t.measurements {
		deltas[i] = m.DeltaMs
		sum += m.DeltaMs
	}
	sort.Slice(deltas, func(i, j int) bool { return deltas[i] < deltas[j] })

	out.Count = len(deltas)
	out.AvgMs = sum / int64(len(deltas))
	out.P50Ms = percentile(deltas, 0.5)
	out.P95Ms = percentile(deltas, 0.95)
	out.LastDeltaMs = t.measurements[len(t.measurements)-1].DeltaMs
	return out
}

// Recent returns up to n most-recent measurements (newest last).
func (t *Tracker) Recent(n int) []Measurement {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.trimWindow(time.Now().UTC())
	if n <= 0 || n > len(t.measurements) {
		n = len(t.measurements)
	}
	out := make([]Measurement, n)
	copy(out, t.measurements[len(t.measurements)-n:])
	return out
}

func (t *Tracker) dropStalePending(now time.Time) {
	cutoff := now.Add(-t.cfg.MaxPairAge)
	i := 0
	for i < len(t.pendingMoves) && t.pendingMoves[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		t.pendingMoves = t.pendingMoves[i:]
	}
}

func (t *Tracker) trimWindow(now time.Time) {
	cutoff := now.Add(-t.cfg.WindowDuration)
	i := 0
	for i < len(t.measurements) && t.measurements[i].PolymarketReprice.Before(cutoff) {
		i++
	}
	if i > 0 {
		t.measurements = t.measurements[i:]
	}
}

func percentile(sorted []int64, p float64) int64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := int(float64(len(sorted)-1) * p)
	return sorted[idx]
}
