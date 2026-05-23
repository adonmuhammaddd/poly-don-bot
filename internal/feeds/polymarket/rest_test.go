package polymarket

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const sampleEventsBody = `[
  {
    "slug": "not-btc",
    "endDate": "2099-01-01T00:00:00Z",
    "closed": false,
    "active": true,
    "markets": [{"question": "other", "clobTokenIds": "[\"a\",\"b\"]"}]
  },
  {
    "slug": "btc-updown-5m-1779501000",
    "endDate": "2099-01-01T00:05:00Z",
    "closed": false,
    "active": true,
    "markets": [{
      "question": "Bitcoin Up or Down - May 22, 9:45PM-9:50PM ET",
      "clobTokenIds": "[\"YES_TOKEN\",\"NO_TOKEN\"]",
      "conditionId": "0xabc"
    }]
  }
]`

func TestRESTClient_FindCurrentMarket_HappyPath(t *testing.T) {
	var receivedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(sampleEventsBody))
	}))
	defer server.Close()

	rc := NewRESTClient(server.URL, 100, testLogger())
	market, err := rc.FindCurrentMarket(context.Background(), "btc-updown-5m-")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if market == nil {
		t.Fatal("got nil market")
	}
	if market.Slug != "btc-updown-5m-1779501000" {
		t.Errorf("slug=%q", market.Slug)
	}
	if market.YesTokenID != "YES_TOKEN" || market.NoTokenID != "NO_TOKEN" {
		t.Errorf("tokens yes=%q no=%q", market.YesTokenID, market.NoTokenID)
	}
	if market.ConditionID != "0xabc" {
		t.Errorf("conditionID=%q", market.ConditionID)
	}
	if !strings.Contains(receivedQuery, "end_date_min=") {
		t.Errorf("query missing end_date_min: %s", receivedQuery)
	}
	if !strings.Contains(receivedQuery, "ascending=true") {
		t.Errorf("query missing ascending=true: %s", receivedQuery)
	}
}

func TestRESTClient_FindCurrentMarket_NoMatch(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[{"slug":"not-btc","markets":[]}]`))
	}))
	defer server.Close()

	rc := NewRESTClient(server.URL, 100, testLogger())
	market, err := rc.FindCurrentMarket(context.Background(), "btc-updown-5m-")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if market != nil {
		t.Errorf("expected nil market, got %+v", market)
	}
}

func TestRESTClient_FindCurrentMarket_HTTPError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	rc := NewRESTClient(server.URL, 100, testLogger())
	_, err := rc.FindCurrentMarket(context.Background(), "btc-updown-5m-")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRESTClient_FindCurrentMarket_SkipsBadTokens(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
		  {"slug":"btc-updown-5m-1","endDate":"2099-01-01T00:00:00Z","markets":[{"question":"a","clobTokenIds":"not-json"}]},
		  {"slug":"btc-updown-5m-2","endDate":"2099-01-01T00:05:00Z","markets":[{"question":"b","clobTokenIds":"[\"y\",\"n\"]","conditionId":"0xab"}]}
		]`))
	}))
	defer server.Close()

	rc := NewRESTClient(server.URL, 100, testLogger())
	market, err := rc.FindCurrentMarket(context.Background(), "btc-updown-5m-")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if market == nil || market.Slug != "btc-updown-5m-2" {
		t.Fatalf("expected slug btc-updown-5m-2, got %+v", market)
	}
}

func TestRESTClient_FindCurrentMarket_RateLimitedDefaultsTo80(t *testing.T) {
	rc := NewRESTClient("http://x", 0, testLogger())
	if rc.limiter.Burst() != 80 {
		t.Errorf("burst=%d want 80", rc.limiter.Burst())
	}
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
