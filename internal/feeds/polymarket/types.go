package polymarket

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/shopspring/decimal"
)

// Market is a discovered active Polymarket market with parsed token IDs.
type Market struct {
	Slug        string
	Question    string
	ConditionID string
	YesTokenID  string
	NoTokenID   string
	EndDate     time.Time
}

// gammaEvent / gammaMarket model the subset of the gamma-api /events response we use.
// The clobTokenIds field arrives as a JSON-encoded string like `"[\"YES\",\"NO\"]"`.
type gammaEvent struct {
	Slug    string        `json:"slug"`
	EndDate string        `json:"endDate"`
	Closed  bool          `json:"closed"`
	Active  bool          `json:"active"`
	Markets []gammaMarket `json:"markets"`
}

type gammaMarket struct {
	Question     string `json:"question"`
	ClobTokenIDs string `json:"clobTokenIds"`
	ConditionID  string `json:"conditionId"`
	EndDate      string `json:"endDate"`
	Closed       bool   `json:"closed"`
	Active       bool   `json:"active"`
}

func (m *gammaMarket) parseTokenIDs() (yes, no string, err error) {
	var ids []string
	if err := json.Unmarshal([]byte(m.ClobTokenIDs), &ids); err != nil {
		return "", "", fmt.Errorf("parse clobTokenIds: %w", err)
	}
	if len(ids) != 2 {
		return "", "", fmt.Errorf("expected 2 token IDs, got %d", len(ids))
	}
	return ids[0], ids[1], nil
}

type subscribeMessage struct {
	AssetsIDs            []string `json:"assets_ids"`
	Type                 string   `json:"type"`
	CustomFeatureEnabled bool     `json:"custom_feature_enabled"`
}

type eventEnvelope struct {
	EventType string `json:"event_type"`
}

// BookEvent matches the `book` payload from the market channel.
type BookEvent struct {
	AssetID   string      `json:"asset_id"`
	Market    string      `json:"market"`
	Bids      []bookLevel `json:"bids"`
	Asks      []bookLevel `json:"asks"`
	Timestamp string      `json:"timestamp"`
	Hash      string      `json:"hash"`
}

type bookLevel struct {
	Price string `json:"price"`
	Size  string `json:"size"`
}

// BestBid returns the highest bid as a decimal.
func (b *BookEvent) BestBid() (decimal.Decimal, bool) {
	return bestLevel(b.Bids, true)
}

// BestAsk returns the lowest ask as a decimal.
func (b *BookEvent) BestAsk() (decimal.Decimal, bool) {
	return bestLevel(b.Asks, false)
}

func bestLevel(levels []bookLevel, max bool) (decimal.Decimal, bool) {
	var best decimal.Decimal
	found := false
	for _, l := range levels {
		p, err := decimal.NewFromString(l.Price)
		if err != nil {
			continue
		}
		if !found || (max && p.GreaterThan(best)) || (!max && p.LessThan(best)) {
			best = p
			found = true
		}
	}
	return best, found
}

// PriceChangeEvent carries delta updates from the market channel.
type PriceChangeEvent struct {
	Market       string        `json:"market"`
	PriceChanges []priceChange `json:"price_changes"`
	Timestamp    string        `json:"timestamp"`
}

type priceChange struct {
	AssetID string `json:"asset_id"`
	Price   string `json:"price"`
	Size    string `json:"size"`
	Side    string `json:"side"`
	Hash    string `json:"hash"`
	BestBid string `json:"best_bid"`
	BestAsk string `json:"best_ask"`
}
