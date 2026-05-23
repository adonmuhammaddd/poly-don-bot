package strategy

import (
	"time"

	"github.com/shopspring/decimal"
)

type Direction string

const (
	DirectionUp   Direction = "up"
	DirectionDown Direction = "down"
)

// Signal is what the detector emits when momentum crosses the threshold.
type Signal struct {
	Symbol     string          `json:"symbol"`
	Direction  Direction       `json:"direction"`
	Magnitude  decimal.Decimal `json:"magnitude"`
	Confidence decimal.Decimal `json:"confidence"`
	DetectedAt time.Time       `json:"detectedAt"`
	WindowMs   int             `json:"windowMs"`
	Context    Context         `json:"context"`
}

// Context is the snapshot of detector state at signal time. Serialised to
// signals.context (JSONB) so we can review or replay later.
type Context struct {
	BinancePrice      decimal.Decimal `json:"binancePrice"`
	WindowStartPrice  decimal.Decimal `json:"windowStartPrice"`
	WindowStartAt     time.Time       `json:"windowStartAt"`
	VelocityBpsPerSec decimal.Decimal `json:"velocityBpsPerSec"`
	TicksInWindow     int             `json:"ticksInWindow"`
}
