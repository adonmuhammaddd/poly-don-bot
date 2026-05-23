package strategy

import (
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

func dec(s string) decimal.Decimal {
	d, err := decimal.NewFromString(s)
	if err != nil {
		panic(err)
	}
	return d
}

func defaultCfg() Config {
	return Config{
		Symbol:           "btcusdt",
		Window:           30 * time.Second,
		Threshold:        dec("0.001"),
		Cooldown:         5 * time.Second,
		MinTicksInWindow: 2,
	}
}

func TestNewDetector_AppliesDefaults(t *testing.T) {
	d := NewDetector(Config{})
	if d.cfg.Window != 30*time.Second {
		t.Errorf("Window default=%v", d.cfg.Window)
	}
	if !d.cfg.Threshold.Equal(dec("0.001")) {
		t.Errorf("Threshold default=%s", d.cfg.Threshold)
	}
	if d.cfg.Cooldown != 5*time.Second {
		t.Errorf("Cooldown default=%v", d.cfg.Cooldown)
	}
	if d.cfg.MinTicksInWindow != 2 {
		t.Errorf("MinTicksInWindow default=%d", d.cfg.MinTicksInWindow)
	}
}

func TestNewDetector_RespectsCustomConfig(t *testing.T) {
	cfg := Config{
		Symbol:           "ethusdt",
		Window:           60 * time.Second,
		Threshold:        dec("0.005"),
		Cooldown:         10 * time.Second,
		MinTicksInWindow: 5,
	}
	d := NewDetector(cfg)
	if d.cfg != cfg {
		t.Errorf("config not preserved: %+v", d.cfg)
	}
}

func TestOnTick_NoSignalBelowMinTicks(t *testing.T) {
	cfg := defaultCfg()
	cfg.MinTicksInWindow = 5
	d := NewDetector(cfg)
	t0 := time.Unix(1700000000, 0).UTC()

	for i := 0; i < 4; i++ {
		sig, ok := d.OnTick(dec("65000").Add(dec("100").Mul(decimal.NewFromInt(int64(i)))), t0.Add(time.Duration(i)*time.Second))
		if ok || sig != nil {
			t.Fatalf("expected no signal at tick %d", i)
		}
	}
}

func TestOnTick_NoSignalBelowThreshold(t *testing.T) {
	d := NewDetector(defaultCfg())
	t0 := time.Unix(1700000000, 0).UTC()
	d.OnTick(dec("65000"), t0)
	// 0.05% move — below 0.1% threshold.
	sig, ok := d.OnTick(dec("65032.50"), t0.Add(time.Second))
	if ok || sig != nil {
		t.Fatalf("expected no signal: ok=%v sig=%+v", ok, sig)
	}
}

func TestOnTick_SignalUp(t *testing.T) {
	d := NewDetector(defaultCfg())
	t0 := time.Unix(1700000000, 0).UTC()
	d.OnTick(dec("65000"), t0)
	d.OnTick(dec("65050"), t0.Add(500*time.Millisecond))
	sig, ok := d.OnTick(dec("65200"), t0.Add(time.Second))

	if !ok || sig == nil {
		t.Fatal("expected signal")
	}
	if sig.Direction != DirectionUp {
		t.Errorf("direction=%s want up", sig.Direction)
	}
	if sig.Symbol != "btcusdt" {
		t.Errorf("symbol=%s", sig.Symbol)
	}
	if !sig.DetectedAt.Equal(t0.Add(time.Second)) {
		t.Errorf("DetectedAt=%v", sig.DetectedAt)
	}
	if sig.WindowMs != 1000 {
		t.Errorf("WindowMs=%d want 1000", sig.WindowMs)
	}
	// magnitude = 200/65000 ≈ 0.003077
	expectedMag := dec("200").Div(dec("65000"))
	if !sig.Magnitude.Equal(expectedMag) {
		t.Errorf("magnitude=%s want %s", sig.Magnitude, expectedMag)
	}
	if !sig.Context.BinancePrice.Equal(dec("65200")) {
		t.Errorf("BinancePrice=%s", sig.Context.BinancePrice)
	}
	if !sig.Context.WindowStartPrice.Equal(dec("65000")) {
		t.Errorf("WindowStartPrice=%s", sig.Context.WindowStartPrice)
	}
	if sig.Context.TicksInWindow != 3 {
		t.Errorf("TicksInWindow=%d", sig.Context.TicksInWindow)
	}
	if !sig.Context.VelocityBpsPerSec.IsPositive() {
		t.Errorf("velocity should be positive: %s", sig.Context.VelocityBpsPerSec)
	}
}

func TestOnTick_SignalDown(t *testing.T) {
	d := NewDetector(defaultCfg())
	t0 := time.Unix(1700000000, 0).UTC()
	d.OnTick(dec("65000"), t0)
	sig, ok := d.OnTick(dec("64800"), t0.Add(time.Second))

	if !ok {
		t.Fatal("expected signal")
	}
	if sig.Direction != DirectionDown {
		t.Errorf("direction=%s want down", sig.Direction)
	}
}

func TestOnTick_ConfidenceClampedAtOne(t *testing.T) {
	d := NewDetector(defaultCfg())
	t0 := time.Unix(1700000000, 0).UTC()
	d.OnTick(dec("65000"), t0)
	// 1% move — way above 2x threshold (0.2%), so confidence should clamp to 1.
	sig, ok := d.OnTick(dec("65650"), t0.Add(time.Second))
	if !ok {
		t.Fatal("expected signal")
	}
	if !sig.Confidence.Equal(decimal.NewFromInt(1)) {
		t.Errorf("confidence=%s want 1", sig.Confidence)
	}
}

func TestOnTick_ConfidenceMidRange(t *testing.T) {
	d := NewDetector(defaultCfg())
	t0 := time.Unix(1700000000, 0).UTC()
	d.OnTick(dec("65000"), t0)
	// 0.15% move → confidence = 0.15 / 0.2 = 0.75
	sig, ok := d.OnTick(dec("65097.50"), t0.Add(time.Second))
	if !ok {
		t.Fatal("expected signal")
	}
	expected := dec("0.75")
	if !sig.Confidence.Equal(expected) {
		t.Errorf("confidence=%s want %s", sig.Confidence, expected)
	}
}

func TestOnTick_CooldownBlocks(t *testing.T) {
	d := NewDetector(defaultCfg())
	t0 := time.Unix(1700000000, 0).UTC()
	d.OnTick(dec("65000"), t0)
	d.OnTick(dec("65200"), t0.Add(time.Second))

	// Within cooldown — should be blocked.
	sig, ok := d.OnTick(dec("65500"), t0.Add(2*time.Second))
	if ok || sig != nil {
		t.Fatalf("expected cooldown block: ok=%v", ok)
	}
}

func TestOnTick_CooldownExpiresAndAllowsNewSignal(t *testing.T) {
	d := NewDetector(defaultCfg())
	t0 := time.Unix(1700000000, 0).UTC()
	d.OnTick(dec("65000"), t0)
	d.OnTick(dec("65200"), t0.Add(time.Second))

	// 6s later — cooldown expired (5s).
	sig, ok := d.OnTick(dec("65500"), t0.Add(7*time.Second))
	if !ok {
		t.Fatal("expected signal after cooldown")
	}
	if sig.Direction != DirectionUp {
		t.Errorf("direction=%s", sig.Direction)
	}
}

func TestOnTick_WindowExpiryDropsOldData(t *testing.T) {
	cfg := defaultCfg()
	cfg.Cooldown = 0 // disable cooldown to isolate window behaviour; falls back to default
	cfg.Window = 10 * time.Second
	d := NewDetector(cfg)
	t0 := time.Unix(1700000000, 0).UTC()

	// Tick at t0 with 65000.
	d.OnTick(dec("65000"), t0)
	// Tick 20s later — window cut off, 65000 should be dropped.
	// Same price — no signal because oldest is now 65500.
	d.OnTick(dec("65500"), t0.Add(20*time.Second))
	sig, ok := d.OnTick(dec("65500"), t0.Add(21*time.Second))
	if ok {
		t.Fatalf("expected no signal after window expiry: %+v", sig)
	}
}

func TestOnTick_WindowDurationZeroSkipsVelocity(t *testing.T) {
	d := NewDetector(defaultCfg())
	t0 := time.Unix(1700000000, 0).UTC()
	// Two ticks at the SAME timestamp with different prices — window duration = 0.
	d.OnTick(dec("65000"), t0)
	sig, ok := d.OnTick(dec("65500"), t0)
	if !ok {
		t.Fatal("expected signal")
	}
	if !sig.Context.VelocityBpsPerSec.IsZero() {
		t.Errorf("velocity should be zero when window duration is zero, got %s", sig.Context.VelocityBpsPerSec)
	}
	if sig.WindowMs != 0 {
		t.Errorf("WindowMs=%d want 0", sig.WindowMs)
	}
}

func TestOnTick_MinTicksInWindowFloorsToTwo(t *testing.T) {
	cfg := defaultCfg()
	cfg.MinTicksInWindow = 0
	d := NewDetector(cfg)
	if d.cfg.MinTicksInWindow != 2 {
		t.Errorf("MinTicksInWindow=%d want 2 (floor)", d.cfg.MinTicksInWindow)
	}
}

func TestOnTick_ExactlyThresholdEmitsNoSignal(t *testing.T) {
	d := NewDetector(defaultCfg())
	t0 := time.Unix(1700000000, 0).UTC()
	d.OnTick(dec("65000"), t0)
	// Exactly 0.1% — threshold is "<=", so no signal.
	sig, ok := d.OnTick(dec("65065"), t0.Add(time.Second))
	if ok || sig != nil {
		t.Fatalf("expected no signal at exact threshold: %+v", sig)
	}
}

func TestOnTick_GoroutineSafe(t *testing.T) {
	d := NewDetector(defaultCfg())
	t0 := time.Unix(1700000000, 0).UTC()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			d.OnTick(dec("65000"), t0.Add(time.Duration(i)*time.Millisecond))
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		d.OnTick(dec("65000"), t0.Add(time.Duration(i+50)*time.Millisecond))
	}
	<-done
}
