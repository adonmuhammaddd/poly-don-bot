package latency

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func TestTracker_DetectsSignificantMove(t *testing.T) {
	tr := NewTracker(Config{
		SignificantMoveThreshold: decimal.NewFromFloat(0.0005),
		MaxPairAge:               30 * time.Second,
		WindowDuration:           60 * time.Second,
	})

	t0 := time.Now().UTC().Add(-5 * time.Second)
	tr.OnTick(decimal.NewFromInt(65000), t0)

	// 0.04% move — below threshold, no pending.
	tr.OnTick(decimal.NewFromFloat(65026), t0.Add(time.Second))
	if got := tr.Stats().PendingMoves; got != 0 {
		t.Errorf("expected 0 pending after sub-threshold, got %d", got)
	}

	// 0.1% move — above threshold.
	tr.OnTick(decimal.NewFromFloat(65091), t0.Add(2*time.Second))
	if got := tr.Stats().PendingMoves; got != 1 {
		t.Errorf("expected 1 pending after move, got %d", got)
	}
}

func TestTracker_PairsMoveWithBookUpdate(t *testing.T) {
	tr := NewTracker(Config{
		SignificantMoveThreshold: decimal.NewFromFloat(0.0005),
		MaxPairAge:               30 * time.Second,
		WindowDuration:           60 * time.Second,
	})

	t0 := time.Now().UTC().Add(-5 * time.Second)
	tr.OnTick(decimal.NewFromInt(65000), t0)
	tr.OnTick(decimal.NewFromFloat(65100), t0.Add(time.Second))

	tr.OnBookUpdate(t0.Add(2 * time.Second))
	stats := tr.Stats()
	if stats.Count != 1 {
		t.Fatalf("count=%d want 1", stats.Count)
	}
	if stats.LastDeltaMs != 1000 {
		t.Errorf("LastDeltaMs=%d want 1000", stats.LastDeltaMs)
	}
}

func TestTracker_DropsStalePending(t *testing.T) {
	tr := NewTracker(Config{
		SignificantMoveThreshold: decimal.NewFromFloat(0.0005),
		MaxPairAge:               5 * time.Second,
		WindowDuration:           60 * time.Second,
	})

	t0 := time.Now().UTC().Add(-5 * time.Second)
	tr.OnTick(decimal.NewFromInt(65000), t0)
	tr.OnTick(decimal.NewFromFloat(65100), t0.Add(time.Second))

	// Book update arrives 10s later — pending is stale.
	tr.OnBookUpdate(t0.Add(11 * time.Second))
	stats := tr.Stats()
	if stats.Count != 0 {
		t.Errorf("count=%d want 0 (stale pending should be dropped)", stats.Count)
	}
}

func TestTracker_PercentileWithMultipleMeasurements(t *testing.T) {
	tr := NewTracker(Config{
		SignificantMoveThreshold: decimal.NewFromFloat(0.0005),
		MaxPairAge:               60 * time.Second,
		WindowDuration:           5 * time.Minute,
	})

	now := time.Now().UTC()
	deltas := []int64{100, 200, 300, 400, 500, 600, 700, 800, 900, 1000}
	price := decimal.NewFromInt(65000)
	tr.OnTick(price, now)
	for i, d := range deltas {
		moveAt := now.Add(time.Duration(i*2) * time.Second)
		price = price.Mul(decimal.NewFromFloat(1.001))
		tr.OnTick(price, moveAt)
		tr.OnBookUpdate(moveAt.Add(time.Duration(d) * time.Millisecond))
	}

	stats := tr.Stats()
	if stats.Count != 10 {
		t.Fatalf("count=%d want 10", stats.Count)
	}
	if stats.AvgMs != 550 {
		t.Errorf("avg=%d want 550", stats.AvgMs)
	}
	if stats.P50Ms != 500 {
		t.Errorf("p50=%d want 500", stats.P50Ms)
	}
	if stats.P95Ms != 900 {
		t.Errorf("p95=%d want 900 (nearest-rank with 10 samples)", stats.P95Ms)
	}
}

func TestTracker_TrimWindow(t *testing.T) {
	tr := NewTracker(Config{
		SignificantMoveThreshold: decimal.NewFromFloat(0.0005),
		MaxPairAge:               5 * time.Minute,
		WindowDuration:           1 * time.Second,
	})

	t0 := time.Now().UTC().Add(-10 * time.Second)
	tr.OnTick(decimal.NewFromInt(65000), t0)
	tr.OnTick(decimal.NewFromFloat(65100), t0.Add(time.Second))
	tr.OnBookUpdate(t0.Add(2 * time.Second))

	stats := tr.Stats()
	if stats.Count != 0 {
		t.Errorf("count=%d want 0 (measurement older than window)", stats.Count)
	}
}

func TestTracker_Recent(t *testing.T) {
	tr := NewTracker(Config{
		SignificantMoveThreshold: decimal.NewFromFloat(0.0005),
		MaxPairAge:               60 * time.Second,
		WindowDuration:           5 * time.Minute,
	})
	now := time.Now().UTC()
	price := decimal.NewFromInt(65000)
	tr.OnTick(price, now)
	for i := 0; i < 5; i++ {
		moveAt := now.Add(time.Duration(i) * time.Second)
		price = price.Mul(decimal.NewFromFloat(1.001))
		tr.OnTick(price, moveAt)
		tr.OnBookUpdate(moveAt.Add(100 * time.Millisecond))
	}
	if got := len(tr.Recent(3)); got != 3 {
		t.Errorf("Recent(3) len=%d want 3", got)
	}
	if got := len(tr.Recent(10)); got != 5 {
		t.Errorf("Recent(10) len=%d want 5 (cap to total)", got)
	}
}
