package observability

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Metrics struct {
	BinanceTicks            *prometheus.CounterVec
	BinanceDisconnects      *prometheus.CounterVec
	BinanceReconnectSeconds prometheus.Histogram
	BinanceMessageLatency   prometheus.Histogram

	PolymarketBookUpdates      *prometheus.CounterVec
	PolymarketDisconnects      *prometheus.CounterVec
	PolymarketReconnectSeconds prometheus.Histogram
	PolymarketActiveMarkets    prometheus.Gauge
	PolymarketRESTErrors       *prometheus.CounterVec

	SignalsDetected     *prometheus.CounterVec
	SignalsPersistError *prometheus.CounterVec

	PaperTradesOpened  *prometheus.CounterVec
	PaperTradesSkipped *prometheus.CounterVec

	registry *prometheus.Registry
}

func NewMetrics() *Metrics {
	reg := prometheus.NewRegistry()
	m := &Metrics{
		BinanceTicks: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "binance_ticks_total",
				Help: "Number of price ticks received from Binance, by symbol and stream.",
			},
			[]string{"symbol", "stream"},
		),
		BinanceDisconnects: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "binance_disconnects_total",
				Help: "Number of WebSocket disconnects from Binance, by reason.",
			},
			[]string{"reason"},
		),
		BinanceReconnectSeconds: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "binance_reconnect_duration_seconds",
				Help:    "Time to re-establish a Binance WebSocket connection after disconnect.",
				Buckets: prometheus.ExponentialBuckets(0.5, 2, 8),
			},
		),
		BinanceMessageLatency: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "binance_message_latency_seconds",
				Help:    "Difference between exchange-reported event time and locally received time.",
				Buckets: prometheus.ExponentialBuckets(0.001, 2, 12),
			},
		),
		PolymarketBookUpdates: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "polymarket_book_updates_total",
				Help: "Number of book/price_change updates received from Polymarket, by side and source.",
			},
			[]string{"side", "source"},
		),
		PolymarketDisconnects: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "polymarket_disconnects_total",
				Help: "Number of WebSocket disconnects from Polymarket, by reason.",
			},
			[]string{"reason"},
		),
		PolymarketReconnectSeconds: prometheus.NewHistogram(
			prometheus.HistogramOpts{
				Name:    "polymarket_reconnect_duration_seconds",
				Help:    "Time to re-establish a Polymarket WebSocket connection after disconnect.",
				Buckets: prometheus.ExponentialBuckets(0.5, 2, 8),
			},
		),
		PolymarketActiveMarkets: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Name: "polymarket_active_markets",
				Help: "1 when a matching BTC Up/Down market is currently being tracked, 0 otherwise.",
			},
		),
		PolymarketRESTErrors: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "polymarket_rest_errors_total",
				Help: "Number of REST API errors from Polymarket gamma-api, by reason.",
			},
			[]string{"reason"},
		),
		SignalsDetected: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signals_detected_total",
				Help: "Number of momentum signals detected, by direction.",
			},
			[]string{"direction"},
		),
		SignalsPersistError: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "signals_persist_errors_total",
				Help: "Number of signal persistence failures, by reason.",
			},
			[]string{"reason"},
		),
		PaperTradesOpened: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "paper_trades_opened_total",
				Help: "Number of paper trades opened, by side (YES/NO).",
			},
			[]string{"side"},
		),
		PaperTradesSkipped: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Name: "paper_trades_skipped_total",
				Help: "Number of signals not converted to paper trades, by reason.",
			},
			[]string{"reason"},
		),
		registry: reg,
	}
	reg.MustRegister(
		m.BinanceTicks,
		m.BinanceDisconnects,
		m.BinanceReconnectSeconds,
		m.BinanceMessageLatency,
		m.PolymarketBookUpdates,
		m.PolymarketDisconnects,
		m.PolymarketReconnectSeconds,
		m.PolymarketActiveMarkets,
		m.PolymarketRESTErrors,
		m.SignalsDetected,
		m.SignalsPersistError,
		m.PaperTradesOpened,
		m.PaperTradesSkipped,
	)
	return m
}

type MetricsServer struct {
	srv *http.Server
}

func NewMetricsServer(port int, m *Metrics) *MetricsServer {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	return &MetricsServer{
		srv: &http.Server{
			Addr:              fmt.Sprintf(":%d", port),
			Handler:           mux,
			ReadHeaderTimeout: 5 * time.Second,
		},
	}
}

func (s *MetricsServer) Run(ctx context.Context) error {
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
