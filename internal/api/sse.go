package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
)

func setupSSEHeaders(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
}

func writeSSE(w http.ResponseWriter, flusher http.Flusher, payload []byte) error {
	if _, err := fmt.Fprintf(w, "data: %s\n\n", payload); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func writeSSEKeepalive(w http.ResponseWriter, flusher http.Flusher) error {
	if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}

func (s *Server) handleStreamPrices(w http.ResponseWriter, r *http.Request) {
	exchange := r.URL.Query().Get("exchange")
	symbol := r.URL.Query().Get("symbol")
	if exchange == "" || symbol == "" {
		http.Error(w, "exchange and symbol required", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	setupSSEHeaders(w)

	ticker := time.NewTicker(s.streamInterval)
	defer ticker.Stop()
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	var lastID int64

	tryPush := func() {
		row, err := s.repo.GetLatestPriceTick(r.Context(), postgres.GetLatestPriceTickParams{
			Exchange: exchange, Symbol: symbol,
		})
		if err != nil {
			return
		}
		if row.ID == lastID {
			return
		}
		lastID = row.ID
		data, err := json.Marshal(priceTickResponseFrom(row))
		if err != nil {
			return
		}
		_ = writeSSE(w, flusher, data)
	}

	tryPush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			tryPush()
		case <-keepalive.C:
			_ = writeSSEKeepalive(w, flusher)
		}
	}
}

func (s *Server) handleStreamLatency(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	setupSSEHeaders(w)

	ticker := time.NewTicker(s.streamInterval)
	defer ticker.Stop()
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	push := func() {
		resp := s.buildLatencyResponse(60)
		data, err := json.Marshal(resp)
		if err != nil {
			return
		}
		_ = writeSSE(w, flusher, data)
	}

	push()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			push()
		case <-keepalive.C:
			_ = writeSSEKeepalive(w, flusher)
		}
	}
}

func (s *Server) handleStreamBook(w http.ResponseWriter, r *http.Request) {
	marketID := r.URL.Query().Get("marketId")
	if marketID == "" {
		http.Error(w, "marketId required", http.StatusBadRequest)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}
	setupSSEHeaders(w)

	ticker := time.NewTicker(s.streamInterval)
	defer ticker.Stop()
	keepalive := time.NewTicker(30 * time.Second)
	defer keepalive.Stop()

	var lastYesUpdated, lastNoUpdated time.Time

	tryPush := func() {
		book, ok := s.latestBook(r, marketID)
		if !ok {
			return
		}
		if book.YesUpdated.Equal(lastYesUpdated) && book.NoUpdated.Equal(lastNoUpdated) {
			return
		}
		lastYesUpdated = book.YesUpdated
		lastNoUpdated = book.NoUpdated
		data, err := json.Marshal(book)
		if err != nil {
			return
		}
		_ = writeSSE(w, flusher, data)
	}

	tryPush()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			tryPush()
		case <-keepalive.C:
			_ = writeSSEKeepalive(w, flusher)
		}
	}
}
