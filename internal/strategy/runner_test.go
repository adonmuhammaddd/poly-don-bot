package strategy

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/shopspring/decimal"
)

type recordingListener struct {
	mu      sync.Mutex
	signals []*Signal
}

func (r *recordingListener) OnSignal(sig *Signal) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.signals = append(r.signals, sig)
}

func TestRunner_FansOutSignal(t *testing.T) {
	det := NewDetector(Config{
		Symbol:    "btcusdt",
		Window:    30 * time.Second,
		Threshold: decimal.NewFromFloat(0.001),
		Cooldown:  5 * time.Second,
	})
	runner := NewRunner(det)
	a := &recordingListener{}
	b := &recordingListener{}
	runner.Subscribe(a)
	runner.Subscribe(b)

	now := time.Unix(1700000000, 0).UTC()
	runner.OnTick(decimal.NewFromInt(65000), now)
	// 0.2% jump triggers a signal.
	runner.OnTick(decimal.NewFromFloat(65130), now.Add(time.Second))

	if len(a.signals) != 1 || len(b.signals) != 1 {
		t.Fatalf("expected one signal per listener, got a=%d b=%d", len(a.signals), len(b.signals))
	}
	if a.signals[0] != b.signals[0] {
		t.Errorf("subscribers should receive the same signal pointer")
	}
}

func TestRunner_NoSignalNoFanout(t *testing.T) {
	det := NewDetector(Config{})
	runner := NewRunner(det)
	listener := &recordingListener{}
	runner.Subscribe(listener)

	now := time.Unix(1700000000, 0).UTC()
	runner.OnTick(decimal.NewFromInt(65000), now)
	if len(listener.signals) != 0 {
		t.Errorf("expected no signals (single tick), got %d", len(listener.signals))
	}
}

func TestRunner_SubscribeAfterFanout(t *testing.T) {
	det := NewDetector(Config{
		Threshold: decimal.NewFromFloat(0.001),
		Cooldown:  100 * time.Millisecond,
	})
	runner := NewRunner(det)

	now := time.Unix(1700000000, 0).UTC()
	runner.OnTick(decimal.NewFromInt(65000), now)
	runner.OnTick(decimal.NewFromFloat(65130), now.Add(time.Second)) // signal emitted, but no listeners

	listener := &recordingListener{}
	runner.Subscribe(listener)
	// New signal after cooldown — listener should receive it.
	runner.OnTick(decimal.NewFromFloat(65300), now.Add(2*time.Second))
	if len(listener.signals) != 1 {
		t.Errorf("expected 1 signal after subscribe, got %d", len(listener.signals))
	}
}

func TestRunner_GoroutineSafeSubscribeAndTick(t *testing.T) {
	det := NewDetector(Config{Cooldown: time.Millisecond})
	runner := NewRunner(det)

	var ticked atomic.Int32
	stop := make(chan struct{})

	go func() {
		now := time.Unix(1700000000, 0).UTC()
		for {
			select {
			case <-stop:
				return
			default:
				runner.OnTick(decimal.NewFromInt(65000), now.Add(time.Duration(ticked.Add(1))*time.Millisecond))
			}
		}
	}()

	for i := 0; i < 20; i++ {
		runner.Subscribe(&recordingListener{})
		time.Sleep(time.Millisecond)
	}
	close(stop)
}
