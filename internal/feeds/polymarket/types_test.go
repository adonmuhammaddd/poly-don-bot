package polymarket

import (
	"testing"

	"github.com/shopspring/decimal"
)

func TestGammaMarket_parseTokenIDs(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantYes string
		wantNo  string
		wantErr bool
	}{
		{"valid", `["100","200"]`, "100", "200", false},
		{"with whitespace", `[ "100" , "200" ]`, "100", "200", false},
		{"three items", `["1","2","3"]`, "", "", true},
		{"one item", `["1"]`, "", "", true},
		{"empty", `[]`, "", "", true},
		{"malformed", `not json`, "", "", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &gammaMarket{ClobTokenIDs: tt.input}
			yes, no, err := m.parseTokenIDs()
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if yes != tt.wantYes {
				t.Errorf("yes=%q want %q", yes, tt.wantYes)
			}
			if no != tt.wantNo {
				t.Errorf("no=%q want %q", no, tt.wantNo)
			}
		})
	}
}

func TestBookEvent_BestBid(t *testing.T) {
	tests := []struct {
		name  string
		bids  []bookLevel
		want  decimal.Decimal
		found bool
	}{
		{"empty", nil, decimal.Zero, false},
		{"single", []bookLevel{{Price: "0.5", Size: "100"}}, decimal.NewFromFloat(0.5), true},
		{"multiple high first", []bookLevel{{Price: "0.6"}, {Price: "0.5"}, {Price: "0.55"}}, decimal.NewFromFloat(0.6), true},
		{"multiple high last", []bookLevel{{Price: "0.5"}, {Price: "0.6"}}, decimal.NewFromFloat(0.6), true},
		{"with invalid price", []bookLevel{{Price: "bad"}, {Price: "0.4"}}, decimal.NewFromFloat(0.4), true},
		{"all invalid", []bookLevel{{Price: "bad"}, {Price: "alsobad"}}, decimal.Zero, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := &BookEvent{Bids: tt.bids}
			got, found := ev.BestBid()
			if found != tt.found {
				t.Errorf("found=%v want %v", found, tt.found)
			}
			if found && !got.Equal(tt.want) {
				t.Errorf("got=%s want %s", got.String(), tt.want.String())
			}
		})
	}
}

func TestBookEvent_BestAsk(t *testing.T) {
	tests := []struct {
		name  string
		asks  []bookLevel
		want  decimal.Decimal
		found bool
	}{
		{"empty", nil, decimal.Zero, false},
		{"single", []bookLevel{{Price: "0.5"}}, decimal.NewFromFloat(0.5), true},
		{"multiple low first", []bookLevel{{Price: "0.5"}, {Price: "0.6"}, {Price: "0.55"}}, decimal.NewFromFloat(0.5), true},
		{"multiple low last", []bookLevel{{Price: "0.6"}, {Price: "0.5"}}, decimal.NewFromFloat(0.5), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ev := &BookEvent{Asks: tt.asks}
			got, found := ev.BestAsk()
			if found != tt.found {
				t.Errorf("found=%v want %v", found, tt.found)
			}
			if found && !got.Equal(tt.want) {
				t.Errorf("got=%s want %s", got.String(), tt.want.String())
			}
		})
	}
}
