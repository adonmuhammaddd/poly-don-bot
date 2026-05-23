package binance

import (
	"encoding/json"
	"time"

	"github.com/shopspring/decimal"
)

// CombinedStreamMessage is the envelope sent on the /stream endpoint:
//
//	wss://stream.binance.com:9443/stream?streams=btcusdt@aggTrade
type CombinedStreamMessage struct {
	Stream string          `json:"stream"`
	Data   json.RawMessage `json:"data"`
}

// AggTradeEvent is the per-trade event under the "aggTrade" stream.
// Reference: https://developers.binance.com/docs/binance-spot-api-docs/web-socket-streams#aggregate-trade-streams
type AggTradeEvent struct {
	EventType        string `json:"e"`
	EventTimeMs      int64  `json:"E"`
	Symbol           string `json:"s"`
	AggregateTradeID int64  `json:"a"`
	Price            string `json:"p"`
	Quantity         string `json:"q"`
	FirstTradeID     int64  `json:"f"`
	LastTradeID      int64  `json:"l"`
	TradeTimeMs      int64  `json:"T"`
	IsBuyerMaker     bool   `json:"m"`
}

func (a *AggTradeEvent) PriceDecimal() (decimal.Decimal, error) {
	return decimal.NewFromString(a.Price)
}

func (a *AggTradeEvent) TradeTime() time.Time {
	return time.UnixMilli(a.TradeTimeMs).UTC()
}
