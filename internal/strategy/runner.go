package strategy

import (
	"sync"
	"time"

	"github.com/shopspring/decimal"
)

// SignalListener observes signals emitted by Runner.
type SignalListener interface {
	OnSignal(sig *Signal)
}

// Runner wraps a Detector and fans out emitted signals to subscribers.
// It implements binance.TickListener so it can plug into the feed pipeline,
// and lets multiple consumers (persister, paper executor, etc.) share a
// single detector window.
type Runner struct {
	detector *Detector

	mu        sync.Mutex
	listeners []SignalListener
}

func NewRunner(detector *Detector) *Runner {
	return &Runner{detector: detector}
}

// Subscribe registers a listener for emitted signals. Goroutine-safe.
func (r *Runner) Subscribe(l SignalListener) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.listeners = append(r.listeners, l)
}

// OnTick delegates to the detector and fans out emitted signals to all
// subscribers. Listeners are invoked outside the lock so a slow listener
// doesn't block subscription operations.
func (r *Runner) OnTick(price decimal.Decimal, ts time.Time) {
	sig, ok := r.detector.OnTick(price, ts)
	if !ok {
		return
	}
	r.mu.Lock()
	snapshot := append([]SignalListener(nil), r.listeners...)
	r.mu.Unlock()
	for _, l := range snapshot {
		l.OnSignal(sig)
	}
}
