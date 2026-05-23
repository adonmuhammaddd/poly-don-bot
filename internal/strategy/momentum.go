package strategy

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// Config tunes the momentum detector. Zero values fall back to sensible defaults.
type Config struct {
	Symbol           string
	Window           time.Duration
	Threshold        decimal.Decimal
	Cooldown         time.Duration
	MinTicksInWindow int
}

type tickSample struct {
	price decimal.Decimal
	ts    time.Time
}

// Detector implements a rolling-window momentum signal detector. It is
// goroutine-safe and holds no I/O — feed it ticks via OnTick.
type Detector struct {
	cfg          Config
	mu           sync.Mutex
	window       []tickSample
	lastSignalAt time.Time
}

func NewDetector(cfg Config) *Detector {
	if cfg.Window <= 0 {
		cfg.Window = 30 * time.Second
	}
	if cfg.Threshold.IsZero() {
		cfg.Threshold = decimal.NewFromFloat(0.001)
	}
	if cfg.Cooldown <= 0 {
		cfg.Cooldown = 5 * time.Second
	}
	if cfg.MinTicksInWindow < 2 {
		cfg.MinTicksInWindow = 2
	}
	return &Detector{cfg: cfg}
}

// OnTick records a Binance price observation. If the rolling-window move
// crosses the threshold and cooldown allows, it returns a populated Signal.
func (d *Detector) OnTick(price decimal.Decimal, ts time.Time) (*Signal, bool) {
	d.mu.Lock()
	defer d.mu.Unlock()

	d.window = append(d.window, tickSample{price: price, ts: ts})
	d.trimWindow(ts)

	if len(d.window) < d.cfg.MinTicksInWindow {
		return nil, false
	}

	if !d.lastSignalAt.IsZero() && ts.Sub(d.lastSignalAt) < d.cfg.Cooldown {
		return nil, false
	}

	oldest := d.window[0]
	delta := price.Sub(oldest.price)
	magnitudeAbs := delta.Abs().Div(oldest.price)
	if magnitudeAbs.LessThanOrEqual(d.cfg.Threshold) {
		return nil, false
	}

	direction := DirectionUp
	if delta.IsNegative() {
		direction = DirectionDown
	}

	windowDur := ts.Sub(oldest.ts)
	var velocity decimal.Decimal
	if windowDur > 0 {
		seconds := decimal.NewFromFloat(windowDur.Seconds())
		velocity = magnitudeAbs.Mul(decimal.NewFromInt(10000)).Div(seconds)
	}

	confidence := magnitudeAbs.Div(d.cfg.Threshold.Mul(decimal.NewFromInt(2)))
	if confidence.GreaterThan(decimal.NewFromInt(1)) {
		confidence = decimal.NewFromInt(1)
	}

	d.lastSignalAt = ts

	return &Signal{
		Symbol:     d.cfg.Symbol,
		Direction:  direction,
		Magnitude:  magnitudeAbs,
		Confidence: confidence,
		DetectedAt: ts,
		WindowMs:   int(windowDur.Milliseconds()),
		Context: Context{
			BinancePrice:      price,
			WindowStartPrice:  oldest.price,
			WindowStartAt:     oldest.ts,
			VelocityBpsPerSec: velocity,
			TicksInWindow:     len(d.window),
		},
	}, true
}

func (d *Detector) trimWindow(now time.Time) {
	cutoff := now.Add(-d.cfg.Window)
	i := 0
	for i < len(d.window) && d.window[i].ts.Before(cutoff) {
		i++
	}
	if i > 0 {
		d.window = d.window[i:]
	}
}
