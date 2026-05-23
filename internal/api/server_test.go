package api

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
)

func TestHandleHealth(t *testing.T) {
	s := newTestServer(&fakeRepo{})
	rec := doRequest(t, s, http.MethodGet, "/api/health")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var got HealthResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.Status != "ok" {
		t.Errorf("status=%q", got.Status)
	}
}

func TestHandleLatestPrice(t *testing.T) {
	now := time.Date(2026, 5, 23, 1, 0, 0, 0, time.UTC)
	tests := []struct {
		name       string
		path       string
		repo       *fakeRepo
		wantStatus int
		wantBody   string
	}{
		{
			"happy",
			"/api/prices/latest?exchange=binance&symbol=btcusdt",
			&fakeRepo{
				latestTick: postgres.GetLatestPriceTickRow{
					ID:         1,
					Exchange:   "binance",
					Symbol:     "btcusdt",
					Price:      decimal.NewFromFloat(65000.5),
					TsExchange: pgtype.Timestamptz{Time: now, Valid: true},
					TsReceived: pgtype.Timestamptz{Time: now, Valid: true},
				},
			},
			http.StatusOK,
			`"price":"65000.5"`,
		},
		{"missing params", "/api/prices/latest", &fakeRepo{}, http.StatusBadRequest, ""},
		{"missing symbol", "/api/prices/latest?exchange=binance", &fakeRepo{}, http.StatusBadRequest, ""},
		{
			"no rows",
			"/api/prices/latest?exchange=binance&symbol=btcusdt",
			&fakeRepo{latestTickErr: pgx.ErrNoRows},
			http.StatusNotFound,
			"",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestServer(tt.repo)
			rec := doRequest(t, s, http.MethodGet, tt.path)
			if rec.Code != tt.wantStatus {
				t.Errorf("status=%d want %d body=%s", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body missing %q: %s", tt.wantBody, rec.Body.String())
			}
		})
	}
}

func TestHandleCurrentMarket(t *testing.T) {
	now := time.Now().UTC()
	repo := &fakeRepo{
		latestMarket: postgres.GetLatestActiveMarketRow{
			MarketID: "0xabc",
			Question: "Bitcoin Up or Down - test",
			LastSeen: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}
	s := newTestServer(repo)
	rec := doRequest(t, s, http.MethodGet, "/api/polymarket/current")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var got CurrentMarketResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.MarketID != "0xabc" {
		t.Errorf("MarketID=%q", got.MarketID)
	}
}

func TestHandleCurrentMarket_NoData(t *testing.T) {
	s := newTestServer(&fakeRepo{latestMarketErr: pgx.ErrNoRows})
	rec := doRequest(t, s, http.MethodGet, "/api/polymarket/current")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status=%d want 404", rec.Code)
	}
}

func TestHandleLatestBook(t *testing.T) {
	bid := decimal.NewFromFloat(0.49)
	ask := decimal.NewFromFloat(0.51)
	noBid := decimal.NewFromFloat(0.48)
	noAsk := decimal.NewFromFloat(0.52)
	now := time.Now().UTC()
	repo := &fakeRepo{
		latestYes: postgres.GetLatestYesQuoteRow{
			YesBid:     decimal.NewNullDecimal(bid),
			YesAsk:     decimal.NewNullDecimal(ask),
			TsReceived: pgtype.Timestamptz{Time: now, Valid: true},
		},
		latestNo: postgres.GetLatestNoQuoteRow{
			NoBid:      decimal.NewNullDecimal(noBid),
			NoAsk:      decimal.NewNullDecimal(noAsk),
			TsReceived: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}
	s := newTestServer(repo)
	rec := doRequest(t, s, http.MethodGet, "/api/polymarket/book/0xabc")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	var got LatestBookResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.YesBid == nil || *got.YesBid != "0.49" {
		t.Errorf("YesBid=%v", got.YesBid)
	}
	if got.Mid == nil || *got.Mid != "0.5000" {
		t.Errorf("Mid=%v", got.Mid)
	}
}

func TestHandleLatestBook_YesOnly(t *testing.T) {
	bid := decimal.NewFromFloat(0.49)
	now := time.Now().UTC()
	repo := &fakeRepo{
		latestYes: postgres.GetLatestYesQuoteRow{
			YesBid:     decimal.NewNullDecimal(bid),
			YesAsk:     decimal.NullDecimal{},
			TsReceived: pgtype.Timestamptz{Time: now, Valid: true},
		},
		latestNoErr: pgx.ErrNoRows,
	}
	s := newTestServer(repo)
	rec := doRequest(t, s, http.MethodGet, "/api/polymarket/book/0xabc")
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	var got LatestBookResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("json: %v", err)
	}
	if got.YesBid == nil || *got.YesBid != "0.49" {
		t.Errorf("YesBid=%v", got.YesBid)
	}
	if got.NoBid != nil {
		t.Errorf("NoBid should be nil: %v", got.NoBid)
	}
	if got.Mid != nil {
		t.Errorf("Mid should be nil when no ask: %v", got.Mid)
	}
}

func TestStreamBook_HeadersAndInitialPush(t *testing.T) {
	bid := decimal.NewFromFloat(0.49)
	ask := decimal.NewFromFloat(0.51)
	now := time.Now().UTC()
	repo := &fakeRepo{
		latestYes: postgres.GetLatestYesQuoteRow{
			YesBid:     decimal.NewNullDecimal(bid),
			YesAsk:     decimal.NewNullDecimal(ask),
			TsReceived: pgtype.Timestamptz{Time: now, Valid: true},
		},
		latestNoErr: pgx.ErrNoRows,
	}
	s := NewServer(Config{Port: 0, StreamInterval: 10 * time.Millisecond}, repo, testLogger())

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/stream/book?marketId=0xabc", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type=%q", resp.Header.Get("Content-Type"))
	}
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])
	if !strings.Contains(body, `"yesBid":"0.49"`) {
		t.Errorf("missing yesBid: %q", body)
	}
}

func TestStreamPrices_MissingParams(t *testing.T) {
	s := newTestServer(&fakeRepo{})
	rec := doRequest(t, s, http.MethodGet, "/api/stream/prices")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rec.Code)
	}
}

func TestStreamBook_MissingParams(t *testing.T) {
	s := newTestServer(&fakeRepo{})
	rec := doRequest(t, s, http.MethodGet, "/api/stream/book")
	if rec.Code != http.StatusBadRequest {
		t.Errorf("status=%d want 400", rec.Code)
	}
}

func TestHandleLatestBook_NoData(t *testing.T) {
	s := newTestServer(&fakeRepo{
		latestYesErr: pgx.ErrNoRows,
		latestNoErr:  pgx.ErrNoRows,
	})
	rec := doRequest(t, s, http.MethodGet, "/api/polymarket/book/0xabc")
	if rec.Code != http.StatusNotFound {
		t.Errorf("status=%d want 404", rec.Code)
	}
}

func TestCORS_PreflightOK(t *testing.T) {
	s := newTestServer(&fakeRepo{})
	rec := doRequest(t, s, http.MethodOptions, "/api/health")
	if rec.Code != http.StatusNoContent {
		t.Errorf("status=%d want 204", rec.Code)
	}
	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("CORS header missing")
	}
}

func TestStreamPrices_HeadersAndInitialPush(t *testing.T) {
	now := time.Date(2026, 5, 23, 1, 0, 0, 0, time.UTC)
	repo := &fakeRepo{
		latestTick: postgres.GetLatestPriceTickRow{
			ID:         42,
			Exchange:   "binance",
			Symbol:     "btcusdt",
			Price:      decimal.NewFromFloat(65000.5),
			TsExchange: pgtype.Timestamptz{Time: now, Valid: true},
			TsReceived: pgtype.Timestamptz{Time: now, Valid: true},
		},
	}
	s := NewServer(Config{Port: 0, StreamInterval: 10 * time.Millisecond}, repo, testLogger())

	ts := httptest.NewServer(s.Handler())
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, ts.URL+"/api/stream/prices?exchange=binance&symbol=btcusdt", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do: %v", err)
	}
	defer resp.Body.Close()

	if resp.Header.Get("Content-Type") != "text/event-stream" {
		t.Errorf("Content-Type=%q", resp.Header.Get("Content-Type"))
	}
	buf := make([]byte, 512)
	n, _ := resp.Body.Read(buf)
	body := string(buf[:n])
	if !strings.Contains(body, "data: ") {
		t.Errorf("missing data prefix: %q", body)
	}
	if !strings.Contains(body, `"price":"65000.5"`) {
		t.Errorf("missing price: %q", body)
	}
}

type fakeRepo struct {
	mu sync.Mutex

	latestTick    postgres.GetLatestPriceTickRow
	latestTickErr error

	latestMarket    postgres.GetLatestActiveMarketRow
	latestMarketErr error

	latestYes    postgres.GetLatestYesQuoteRow
	latestYesErr error

	latestNo    postgres.GetLatestNoQuoteRow
	latestNoErr error
}

func (f *fakeRepo) GetLatestPriceTick(_ context.Context, _ postgres.GetLatestPriceTickParams) (postgres.GetLatestPriceTickRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.latestTick, f.latestTickErr
}

func (f *fakeRepo) GetLatestActiveMarket(_ context.Context, _ pgtype.Timestamptz) (postgres.GetLatestActiveMarketRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.latestMarket, f.latestMarketErr
}

func (f *fakeRepo) GetLatestYesQuote(_ context.Context, _ string) (postgres.GetLatestYesQuoteRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.latestYes, f.latestYesErr
}

func (f *fakeRepo) GetLatestNoQuote(_ context.Context, _ string) (postgres.GetLatestNoQuoteRow, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.latestNo, f.latestNoErr
}

func newTestServer(repo Repository) *Server {
	return NewServer(Config{Port: 0, StreamInterval: 10 * time.Millisecond}, repo, testLogger())
}

func doRequest(t *testing.T, s *Server, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, req)
	return rec
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
