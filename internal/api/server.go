package api

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/adonmuhammaddd/poly-don-bot/internal/latency"
	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
)

// LatencyView exposes the read surface the API needs from the tracker.
type LatencyView interface {
	Stats() latency.Stats
	Recent(n int) []latency.Measurement
}

type Repository interface {
	GetLatestPriceTick(ctx context.Context, arg postgres.GetLatestPriceTickParams) (postgres.GetLatestPriceTickRow, error)
	GetLatestActiveMarket(ctx context.Context, since pgtype.Timestamptz) (postgres.GetLatestActiveMarketRow, error)
	GetLatestYesQuote(ctx context.Context, marketID string) (postgres.GetLatestYesQuoteRow, error)
	GetLatestNoQuote(ctx context.Context, marketID string) (postgres.GetLatestNoQuoteRow, error)
}

type Config struct {
	Port           int
	StreamInterval time.Duration
	AllowOrigin    string
}

type Server struct {
	repo           Repository
	latency        LatencyView
	logger         *slog.Logger
	srv            *http.Server
	streamInterval time.Duration
	router         http.Handler
}

func NewServer(cfg Config, repo Repository, lat LatencyView, logger *slog.Logger) *Server {
	if cfg.StreamInterval == 0 {
		cfg.StreamInterval = 250 * time.Millisecond
	}
	if cfg.AllowOrigin == "" {
		cfg.AllowOrigin = "*"
	}

	s := &Server{
		repo:           repo,
		latency:        lat,
		logger:         logger.With(slog.String("component", "api")),
		streamInterval: cfg.StreamInterval,
	}

	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(corsMiddleware(cfg.AllowOrigin))
	r.Use(s.slogMiddleware)

	r.Get("/api/health", s.handleHealth)
	r.Get("/api/prices/latest", s.handleLatestPrice)
	r.Get("/api/polymarket/current", s.handleCurrentMarket)
	r.Get("/api/polymarket/book/{marketId}", s.handleLatestBook)
	r.Get("/api/latency/recent", s.handleLatencyRecent)
	r.Get("/api/stream/prices", s.handleStreamPrices)
	r.Get("/api/stream/book", s.handleStreamBook)
	r.Get("/api/stream/latency", s.handleStreamLatency)

	s.router = r
	s.srv = &http.Server{
		Addr:              fmt.Sprintf(":%d", cfg.Port),
		Handler:           r,
		ReadHeaderTimeout: 5 * time.Second,
	}
	return s
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		err := s.srv.ListenAndServe()
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return s.srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		return err
	}
}

// Handler exposes the router so tests can call it directly.
func (s *Server) Handler() http.Handler {
	return s.router
}

func corsMiddleware(allow string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", allow)
			w.Header().Set("Access-Control-Allow-Methods", "GET, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
			if r.Method == http.MethodOptions {
				w.WriteHeader(http.StatusNoContent)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (s *Server) slogMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		s.logger.Info("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", ww.Status()),
			slog.Duration("duration", time.Since(start)),
			slog.String("request_id", middleware.GetReqID(r.Context())),
		)
	})
}
