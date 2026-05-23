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
		registry: reg,
	}
	reg.MustRegister(
		m.BinanceTicks,
		m.BinanceDisconnects,
		m.BinanceReconnectSeconds,
		m.BinanceMessageLatency,
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
