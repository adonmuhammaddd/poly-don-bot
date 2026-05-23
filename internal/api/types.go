package api

import (
	"encoding/json"
	"time"

	"github.com/adonmuhammaddd/poly-don-bot/internal/latency"
)

type HealthResponse struct {
	Status string `json:"status"`
}

type PriceTickResponse struct {
	Exchange   string    `json:"exchange"`
	Symbol     string    `json:"symbol"`
	Price      string    `json:"price"`
	TsExchange time.Time `json:"tsExchange"`
	TsReceived time.Time `json:"tsReceived"`
}

type CurrentMarketResponse struct {
	MarketID string    `json:"marketId"`
	Question string    `json:"question"`
	LastSeen time.Time `json:"lastSeen"`
}

type LatestBookResponse struct {
	MarketID   string    `json:"marketId"`
	YesBid     *string   `json:"yesBid,omitempty"`
	YesAsk     *string   `json:"yesAsk,omitempty"`
	NoBid      *string   `json:"noBid,omitempty"`
	NoAsk      *string   `json:"noAsk,omitempty"`
	YesUpdated time.Time `json:"yesUpdated"`
	NoUpdated  time.Time `json:"noUpdated"`
	Mid        *string   `json:"mid,omitempty"`
}

type LatencyResponse struct {
	Stats   latency.Stats        `json:"stats"`
	Samples []LatencyMeasurement `json:"samples"`
}

type LatencyMeasurement struct {
	BinanceMoveAt     time.Time `json:"binanceMoveAt"`
	PolymarketReprice time.Time `json:"polymarketReprice"`
	DeltaMs           int64     `json:"deltaMs"`
}

type SignalResponse struct {
	ID           int64           `json:"id"`
	Symbol       string          `json:"symbol"`
	Direction    string          `json:"direction"`
	Magnitude    string          `json:"magnitude"`
	WindowMs     int32           `json:"windowMs"`
	Confidence   string          `json:"confidence"`
	DetectedAt   time.Time       `json:"detectedAt"`
	Context      json.RawMessage `json:"context"`
	ActionTaken  string          `json:"actionTaken,omitempty"`
	ActionReason string          `json:"actionReason,omitempty"`
}
