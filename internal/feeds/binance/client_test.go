package binance

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"github.com/adonmuhammaddd/poly-don-bot/internal/observability"
	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
)

const sampleAggTradeMessage = `{"stream":"btcusdt@aggTrade","data":{"e":"aggTrade","E":1234567890123,"s":"BTCUSDT","a":99,"p":"65000.50","q":"0.1","f":1,"l":2,"T":1234567890000,"m":false}}`

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		name string
		cur  time.Duration
		max  time.Duration
		want time.Duration
	}{
		{"first growth", 1 * time.Second, 30 * time.Second, 2 * time.Second},
		{"mid growth", 8 * time.Second, 30 * time.Second, 16 * time.Second},
		{"about to cap", 20 * time.Second, 30 * time.Second, 30 * time.Second},
		{"already at cap", 30 * time.Second, 30 * time.Second, 30 * time.Second},
		{"overshoot capped", 25 * time.Second, 30 * time.Second, 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := NextBackoff(tt.cur, tt.max)
			if got != tt.want {
				t.Errorf("NextBackoff(%v, %v) = %v, want %v", tt.cur, tt.max, got, tt.want)
			}
		})
	}
}

func TestBuildStreamURL(t *testing.T) {
	tests := []struct {
		name    string
		base    string
		symbols []string
		streams []string
		want    string
	}{
		{
			"single stream",
			"wss://stream.binance.com:9443/stream",
			[]string{"btcusdt"},
			[]string{"aggTrade"},
			"wss://stream.binance.com:9443/stream?streams=btcusdt%40aggTrade",
		},
		{
			"uppercase symbol lowercased",
			"wss://stream.binance.com:9443/stream",
			[]string{"BTCUSDT"},
			[]string{"aggTrade"},
			"wss://stream.binance.com:9443/stream?streams=btcusdt%40aggTrade",
		},
		{
			"multi symbol multi stream",
			"wss://stream.binance.com:9443/stream",
			[]string{"btcusdt", "ethusdt"},
			[]string{"aggTrade", "bookTicker"},
			"wss://stream.binance.com:9443/stream?streams=btcusdt%40aggTrade%2Fbtcusdt%40bookTicker%2Fethusdt%40aggTrade%2Fethusdt%40bookTicker",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := buildStreamURL(tt.base, tt.symbols, tt.streams)
			if err != nil {
				t.Fatalf("buildStreamURL: %v", err)
			}
			if got != tt.want {
				t.Errorf("buildStreamURL\ngot:  %s\nwant: %s", got, tt.want)
			}
		})
	}
}

func TestClassifyError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"nil", nil, "unknown"},
		{"context canceled", context.Canceled, "context_canceled"},
		{"deadline", context.DeadlineExceeded, "deadline"},
		{"close normal", &websocket.CloseError{Code: 1000}, "close_1000"},
		{"close abnormal", &websocket.CloseError{Code: 1006}, "close_1006"},
		{"generic io", io.ErrUnexpectedEOF, "io_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := classifyError(tt.err)
			if got != tt.want {
				t.Errorf("classifyError(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestAggTradeEvent_Parsing(t *testing.T) {
	raw := []byte(`{"e":"aggTrade","E":1234567890123,"s":"BTCUSDT","a":99,"p":"65000.50","q":"0.1","f":1,"l":2,"T":1234567890000,"m":false}`)
	var ev AggTradeEvent
	if err := json.Unmarshal(raw, &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.Symbol != "BTCUSDT" {
		t.Errorf("Symbol = %q, want BTCUSDT", ev.Symbol)
	}
	price, err := ev.PriceDecimal()
	if err != nil {
		t.Fatalf("PriceDecimal: %v", err)
	}
	if price.String() != "65000.5" {
		t.Errorf("Price = %s, want 65000.5", price.String())
	}
	tm := ev.TradeTime()
	if tm.UnixMilli() != 1234567890000 {
		t.Errorf("TradeTime ms = %d, want 1234567890000", tm.UnixMilli())
	}
	if tm.Location() != time.UTC {
		t.Errorf("TradeTime location = %v, want UTC", tm.Location())
	}
}

func TestHandleMessage_PersistsAggTrade(t *testing.T) {
	fake := &fakeStorage{}
	client := newTestClient(fake)

	if err := client.handleMessage(context.Background(), []byte(sampleAggTradeMessage)); err != nil {
		t.Fatalf("handleMessage: %v", err)
	}
	inserts := fake.Inserts()
	if len(inserts) != 1 {
		t.Fatalf("got %d inserts, want 1", len(inserts))
	}
	ins := inserts[0]
	if ins.Exchange != "binance" {
		t.Errorf("Exchange = %q, want binance", ins.Exchange)
	}
	if ins.Symbol != "btcusdt" {
		t.Errorf("Symbol = %q, want btcusdt", ins.Symbol)
	}
	if ins.Price.String() != "65000.5" {
		t.Errorf("Price = %s, want 65000.5", ins.Price.String())
	}
	if !ins.TsExchange.Valid {
		t.Error("TsExchange not valid")
	}
	if ins.TsExchange.Time.UnixMilli() != 1234567890000 {
		t.Errorf("TsExchange ms = %d, want 1234567890000", ins.TsExchange.Time.UnixMilli())
	}
}

func TestHandleMessage_IgnoresNonAggTrade(t *testing.T) {
	fake := &fakeStorage{}
	client := newTestClient(fake)

	raw := []byte(`{"stream":"btcusdt@bookTicker","data":{"u":1,"s":"BTCUSDT","b":"65000","B":"1","a":"65001","A":"1"}}`)
	if err := client.handleMessage(context.Background(), raw); err != nil {
		t.Fatalf("handleMessage: %v", err)
	}
	if got := len(fake.Inserts()); got != 0 {
		t.Errorf("got %d inserts, want 0", got)
	}
}

func TestHandleMessage_BadEnvelopeReturnsError(t *testing.T) {
	fake := &fakeStorage{}
	client := newTestClient(fake)
	err := client.handleMessage(context.Background(), []byte("not json"))
	if err == nil {
		t.Fatal("want error, got nil")
	}
}

func TestRunOnce_DialFails(t *testing.T) {
	fake := &fakeStorage{}
	client := newTestClient(fake)
	client.WithDialer(errDialer{})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := client.runOnce(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestClient_EndToEnd(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}

	shutdown := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.TextMessage, []byte(sampleAggTradeMessage)); err != nil {
					return
				}
			case <-shutdown:
				return
			}
		}
	}))
	defer server.Close()
	defer close(shutdown)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/stream"

	fake := &fakeStorage{}
	metrics := observability.NewMetrics()
	client := NewClient(Config{
		WSURL:             wsURL,
		Symbols:           []string{"btcusdt"},
		Streams:           []string{"aggTrade"},
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  2 * time.Second,
	}, testLogger(), metrics, fake)

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = client.Run(ctx)
	}()

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		if len(fake.Inserts()) > 0 {
			break
		}
		select {
		case <-deadline.C:
			t.Fatal("no inserts received within 2s")
		case <-time.After(20 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("client did not stop within 1s of cancel")
	}

	ins := fake.Inserts()[0]
	if ins.Symbol != "btcusdt" {
		t.Errorf("Symbol = %q, want btcusdt", ins.Symbol)
	}
	if ins.Price.String() != "65000.5" {
		t.Errorf("Price = %s, want 65000.5", ins.Price.String())
	}
}

func TestClient_ReconnectsAfterDisconnect(t *testing.T) {
	upgrader := websocket.Upgrader{CheckOrigin: func(_ *http.Request) bool { return true }}
	var connects atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := connects.Add(1)
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		defer conn.Close()
		_ = conn.WriteMessage(websocket.TextMessage, []byte(sampleAggTradeMessage))
		if n == 1 {
			return
		}
		time.Sleep(500 * time.Millisecond)
	}))
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/stream"

	fake := &fakeStorage{}
	metrics := observability.NewMetrics()
	client := NewClient(Config{
		WSURL:             wsURL,
		Symbols:           []string{"btcusdt"},
		Streams:           []string{"aggTrade"},
		InitialBackoff:    10 * time.Millisecond,
		MaxBackoff:        100 * time.Millisecond,
		HeartbeatInterval: 5 * time.Second,
		HeartbeatTimeout:  2 * time.Second,
	}, testLogger(), metrics, fake)

	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		defer close(runDone)
		_ = client.Run(ctx)
	}()

	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		if connects.Load() >= 2 && len(fake.Inserts()) >= 2 {
			break
		}
		select {
		case <-deadline.C:
			t.Fatalf("only %d connects, %d inserts", connects.Load(), len(fake.Inserts()))
		case <-time.After(20 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("client did not stop within 1s of cancel")
	}
}

type fakeStorage struct {
	mu      sync.Mutex
	inserts []postgres.InsertPriceTickParams
	err     error
}

func (f *fakeStorage) InsertPriceTick(_ context.Context, arg postgres.InsertPriceTickParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return 0, f.err
	}
	f.inserts = append(f.inserts, arg)
	return int64(len(f.inserts)), nil
}

func (f *fakeStorage) Inserts() []postgres.InsertPriceTickParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]postgres.InsertPriceTickParams, len(f.inserts))
	copy(out, f.inserts)
	return out
}

type errDialer struct{}

func (errDialer) DialContext(_ context.Context, _ string, _ http.Header) (*websocket.Conn, *http.Response, error) {
	return nil, nil, errors.New("test dial failure")
}

func newTestClient(s Storage) *Client {
	return NewClient(Config{
		InitialBackoff:    time.Millisecond,
		MaxBackoff:        10 * time.Millisecond,
		HeartbeatInterval: time.Second,
		HeartbeatTimeout:  time.Second,
	}, testLogger(), observability.NewMetrics(), s)
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}
