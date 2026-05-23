package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
)

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, HealthResponse{Status: "ok"})
}

func (s *Server) handleLatestPrice(w http.ResponseWriter, r *http.Request) {
	exchange := r.URL.Query().Get("exchange")
	symbol := r.URL.Query().Get("symbol")
	if exchange == "" || symbol == "" {
		http.Error(w, "exchange and symbol required", http.StatusBadRequest)
		return
	}
	row, err := s.repo.GetLatestPriceTick(r.Context(), postgres.GetLatestPriceTickParams{
		Exchange: exchange,
		Symbol:   symbol,
	})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "no data", http.StatusNotFound)
			return
		}
		s.logger.Error("get latest price tick", slog.Any("error", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, priceTickResponseFrom(row))
}

func (s *Server) handleCurrentMarket(w http.ResponseWriter, r *http.Request) {
	since := pgtype.Timestamptz{Time: time.Now().UTC().Add(-5 * time.Minute), Valid: true}
	row, err := s.repo.GetLatestActiveMarket(r.Context(), since)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "no active market", http.StatusNotFound)
			return
		}
		s.logger.Error("get latest active market", slog.Any("error", err))
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, CurrentMarketResponse{
		MarketID: row.MarketID,
		Question: row.Question,
		LastSeen: row.LastSeen.Time.UTC(),
	})
}

func (s *Server) handleLatestBook(w http.ResponseWriter, r *http.Request) {
	marketID := chi.URLParam(r, "marketId")
	if marketID == "" {
		http.Error(w, "marketId required", http.StatusBadRequest)
		return
	}
	resp, ok := s.latestBook(r, marketID)
	if !ok {
		http.Error(w, "no data", http.StatusNotFound)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) latestBook(r *http.Request, marketID string) (LatestBookResponse, bool) {
	yes, yesErr := s.repo.GetLatestYesQuote(r.Context(), marketID)
	no, noErr := s.repo.GetLatestNoQuote(r.Context(), marketID)

	if errors.Is(yesErr, pgx.ErrNoRows) && errors.Is(noErr, pgx.ErrNoRows) {
		return LatestBookResponse{}, false
	}
	if yesErr != nil && !errors.Is(yesErr, pgx.ErrNoRows) {
		s.logger.Error("get yes quote", slog.Any("error", yesErr))
	}
	if noErr != nil && !errors.Is(noErr, pgx.ErrNoRows) {
		s.logger.Error("get no quote", slog.Any("error", noErr))
	}

	resp := LatestBookResponse{MarketID: marketID}
	if yesErr == nil {
		if yes.YesBid.Valid {
			v := yes.YesBid.Decimal.String()
			resp.YesBid = &v
		}
		if yes.YesAsk.Valid {
			v := yes.YesAsk.Decimal.String()
			resp.YesAsk = &v
		}
		resp.YesUpdated = yes.TsReceived.Time.UTC()
	}
	if noErr == nil {
		if no.NoBid.Valid {
			v := no.NoBid.Decimal.String()
			resp.NoBid = &v
		}
		if no.NoAsk.Valid {
			v := no.NoAsk.Decimal.String()
			resp.NoAsk = &v
		}
		resp.NoUpdated = no.TsReceived.Time.UTC()
	}

	if resp.YesBid != nil && resp.YesAsk != nil {
		bid, errBid := decimal.NewFromString(*resp.YesBid)
		ask, errAsk := decimal.NewFromString(*resp.YesAsk)
		if errBid == nil && errAsk == nil {
			mid := bid.Add(ask).Div(decimal.NewFromInt(2)).StringFixed(4)
			resp.Mid = &mid
		}
	}

	return resp, true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func priceTickResponseFrom(row postgres.GetLatestPriceTickRow) PriceTickResponse {
	return PriceTickResponse{
		Exchange:   row.Exchange,
		Symbol:     row.Symbol,
		Price:      row.Price.String(),
		TsExchange: row.TsExchange.Time.UTC(),
		TsReceived: row.TsReceived.Time.UTC(),
	}
}
