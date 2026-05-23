package polymarket

import (
	"context"
	"errors"
	"io"
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

const sampleBookEventYES = `{"event_type":"book","asset_id":"YES_TOKEN","market":"0xmarket","bids":[{"price":"0.48","size":"30"},{"price":"0.49","size":"20"}],"asks":[{"price":"0.52","size":"25"},{"price":"0.53","size":"60"}],"timestamp":"1779501000000","hash":"0xabc"}`
const sampleBookEventNO = `{"event_type":"book","asset_id":"NO_TOKEN","market":"0xmarket","bids":[{"price":"0.46","size":"30"}],"asks":[{"price":"0.51","size":"25"}],"timestamp":"1779501000000","hash":"0xdef"}`
const samplePriceChangeYES = `{"event_type":"price_change","market":"0xmarket","price_changes":[{"asset_id":"YES_TOKEN","price":"0.5","size":"200","side":"BUY","hash":"h","best_bid":"0.49","best_ask":"0.51"}],"timestamp":"1779501000000"}`

func TestNextBackoff(t *testing.T) {
	tests := []struct {
		name string
		cur  time.Duration
		max  time.Duration
		want time.Duration
	}{
		{"first growth", 1 * time.Second, 30 * time.Second, 2 * time.Second},
		{"cap reached", 25 * time.Second, 30 * time.Second, 30 * time.Second},
		{"already at cap", 30 * time.Second, 30 * time.Second, 30 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NextBackoff(tt.cur, tt.max); got != tt.want {
				t.Errorf("got %v want %v", got, tt.want)
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
		{"canceled", context.Canceled, "context_canceled"},
		{"deadline", context.DeadlineExceeded, "deadline"},
		{"close 1000", &websocket.CloseError{Code: 1000}, "close_1000"},
		{"io", io.ErrUnexpectedEOF, "io_error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := classifyError(tt.err); got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestSideForAsset(t *testing.T) {
	market := &Market{YesTokenID: "YES_TOKEN", NoTokenID: "NO_TOKEN"}
	tests := []struct {
		asset string
		want  string
	}{
		{"YES_TOKEN", "yes"},
		{"NO_TOKEN", "no"},
		{"OTHER", ""},
		{"", ""},
	}
	for _, tt := range tests {
		t.Run(tt.asset, func(t *testing.T) {
			if got := sideForAsset(tt.asset, market); got != tt.want {
				t.Errorf("got %q want %q", got, tt.want)
			}
		})
	}
}

func TestHandleBookEvent_PersistsYES(t *testing.T) {
	market := &Market{ConditionID: "0xmarket", Question: "Test", YesTokenID: "YES_TOKEN", NoTokenID: "NO_TOKEN"}
	fake := &fakeStorage{}
	client := newTestClient(fake, nil)

	if err := client.handleMessage(context.Background(), market, []byte(sampleBookEventYES)); err != nil {
		t.Fatalf("handleMessage: %v", err)
	}
	inserts := fake.Inserts()
	if len(inserts) != 1 {
		t.Fatalf("got %d inserts want 1", len(inserts))
	}
	ins := inserts[0]
	if ins.MarketID != "0xmarket" {
		t.Errorf("MarketID=%q", ins.MarketID)
	}
	if !ins.YesBid.Valid || ins.YesBid.Decimal.String() != "0.49" {
		t.Errorf("YesBid=%v", ins.YesBid)
	}
	if !ins.YesAsk.Valid || ins.YesAsk.Decimal.String() != "0.52" {
		t.Errorf("YesAsk=%v", ins.YesAsk)
	}
	if ins.NoBid.Valid || ins.NoAsk.Valid {
		t.Errorf("NO columns should be null: bid=%v ask=%v", ins.NoBid, ins.NoAsk)
	}
}

func TestHandleBookEvent_PersistsNO(t *testing.T) {
	market := &Market{ConditionID: "0xmarket", YesTokenID: "YES_TOKEN", NoTokenID: "NO_TOKEN"}
	fake := &fakeStorage{}
	client := newTestClient(fake, nil)

	if err := client.handleMessage(context.Background(), market, []byte(sampleBookEventNO)); err != nil {
		t.Fatalf("handleMessage: %v", err)
	}
	inserts := fake.Inserts()
	if len(inserts) != 1 {
		t.Fatalf("got %d inserts want 1", len(inserts))
	}
	ins := inserts[0]
	if !ins.NoBid.Valid || ins.NoBid.Decimal.String() != "0.46" {
		t.Errorf("NoBid=%v", ins.NoBid)
	}
	if !ins.NoAsk.Valid || ins.NoAsk.Decimal.String() != "0.51" {
		t.Errorf("NoAsk=%v", ins.NoAsk)
	}
	if ins.YesBid.Valid || ins.YesAsk.Valid {
		t.Errorf("YES columns should be null")
	}
}

func TestHandlePriceChange(t *testing.T) {
	market := &Market{ConditionID: "0xmarket", YesTokenID: "YES_TOKEN", NoTokenID: "NO_TOKEN"}
	fake := &fakeStorage{}
	client := newTestClient(fake, nil)

	if err := client.handleMessage(context.Background(), market, []byte(samplePriceChangeYES)); err != nil {
		t.Fatalf("handleMessage: %v", err)
	}
	inserts := fake.Inserts()
	if len(inserts) != 1 {
		t.Fatalf("got %d inserts want 1", len(inserts))
	}
	ins := inserts[0]
	if !ins.YesBid.Valid || ins.YesBid.Decimal.String() != "0.49" {
		t.Errorf("YesBid=%v", ins.YesBid)
	}
	if !ins.YesAsk.Valid || ins.YesAsk.Decimal.String() != "0.51" {
		t.Errorf("YesAsk=%v", ins.YesAsk)
	}
}

func TestHandleMessage_PongIgnored(t *testing.T) {
	market := &Market{YesTokenID: "Y", NoTokenID: "N"}
	fake := &fakeStorage{}
	client := newTestClient(fake, nil)
	if err := client.handleMessage(context.Background(), market, []byte("PONG")); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(fake.Inserts()) != 0 {
		t.Errorf("PONG should not insert")
	}
}

func TestHandleMessage_UnknownEventType(t *testing.T) {
	market := &Market{YesTokenID: "Y", NoTokenID: "N"}
	fake := &fakeStorage{}
	client := newTestClient(fake, nil)
	raw := []byte(`{"event_type":"last_trade_price","asset_id":"Y","price":"0.5"}`)
	if err := client.handleMessage(context.Background(), market, raw); err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if len(fake.Inserts()) != 0 {
		t.Errorf("unknown event should not insert")
	}
}

func TestHandleMessage_Array(t *testing.T) {
	market := &Market{ConditionID: "0xmarket", YesTokenID: "YES_TOKEN", NoTokenID: "NO_TOKEN"}
	fake := &fakeStorage{}
	client := newTestClient(fake, nil)
	batch := []byte("[" + sampleBookEventYES + "," + sampleBookEventNO + "]")
	if err := client.handleMessage(context.Background(), market, batch); err != nil {
		t.Fatalf("handleMessage: %v", err)
	}
	if got := len(fake.Inserts()); got != 2 {
		t.Errorf("got %d inserts want 2", got)
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

		// First, server must read the subscribe message client sends.
		if _, _, err := conn.ReadMessage(); err != nil {
			return
		}

		ticker := time.NewTicker(50 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := conn.WriteMessage(websocket.TextMessage, []byte(sampleBookEventYES)); err != nil {
					return
				}
			case <-shutdown:
				return
			}
		}
	}))
	defer server.Close()
	defer close(shutdown)

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")

	finder := fakeFinder{
		market: &Market{
			Slug:        "btc-updown-5m-test",
			Question:    "test",
			ConditionID: "0xmarket",
			YesTokenID:  "YES_TOKEN",
			NoTokenID:   "NO_TOKEN",
			EndDate:     time.Now().Add(5 * time.Minute),
		},
	}
	fake := &fakeStorage{}
	client := newTestClient(fake, &finder)
	client.cfg.WSURL = wsURL

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
			t.Fatal("no inserts within 2s")
		case <-time.After(20 * time.Millisecond):
		}
	}

	cancel()
	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("client did not stop within 1s")
	}
}

type fakeStorage struct {
	mu      sync.Mutex
	inserts []postgres.InsertPolymarketBookParams
}

func (f *fakeStorage) InsertPolymarketBook(_ context.Context, arg postgres.InsertPolymarketBookParams) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inserts = append(f.inserts, arg)
	return int64(len(f.inserts)), nil
}

func (f *fakeStorage) Inserts() []postgres.InsertPolymarketBookParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]postgres.InsertPolymarketBookParams, len(f.inserts))
	copy(out, f.inserts)
	return out
}

type fakeFinder struct {
	mu     sync.Mutex
	calls  atomic.Int32
	market *Market
	err    error
}

func (f *fakeFinder) FindCurrentMarket(_ context.Context, _ string) (*Market, error) {
	f.calls.Add(1)
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.market, f.err
}

func newTestClient(s Storage, finder MarketFinder) *Client {
	c := NewClient(Config{
		InitialBackoff: 10 * time.Millisecond,
		MaxBackoff:     100 * time.Millisecond,
		PingInterval:   5 * time.Second,
		ReadTimeout:    2 * time.Second,
	}, testLogger(), observability.NewMetrics(), s)
	if finder != nil {
		c.WithMarketFinder(finder)
	}
	return c
}

var _ = errors.New
