package main

import (
	"context"
	"encoding/csv"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"

	"github.com/adonmuhammaddd/poly-don-bot/internal/config"
	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
	"github.com/adonmuhammaddd/poly-don-bot/internal/strategy"
)

func main() {
	fromStr := flag.String("from", "", "start time, RFC3339 (e.g. 2026-05-23T00:00:00Z)")
	toStr := flag.String("to", "", "end time, RFC3339")
	exchange := flag.String("exchange", "binance", "exchange filter")
	symbol := flag.String("symbol", "btcusdt", "symbol filter")
	thresholdStr := flag.String("threshold", "0.001", "magnitude threshold (0.001 = 0.1%)")
	window := flag.Duration("window", 30*time.Second, "detector rolling window")
	cooldown := flag.Duration("cooldown", 5*time.Second, "cooldown between signals")
	format := flag.String("format", "summary", "output: summary | csv | json")
	flag.Parse()

	if *fromStr == "" || *toStr == "" {
		fmt.Fprintln(os.Stderr, "-from and -to are required")
		flag.Usage()
		os.Exit(2)
	}
	from, err := time.Parse(time.RFC3339, *fromStr)
	if err != nil {
		log.Fatalf("parse from: %v", err)
	}
	to, err := time.Parse(time.RFC3339, *toStr)
	if err != nil {
		log.Fatalf("parse to: %v", err)
	}
	if !to.After(from) {
		log.Fatal("-to must be after -from")
	}
	threshold, err := decimal.NewFromString(*thresholdStr)
	if err != nil {
		log.Fatalf("parse threshold: %v", err)
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	ctx := context.Background()
	pool, err := postgres.NewPool(ctx, cfg.Postgres.URL)
	if err != nil {
		log.Fatalf("postgres: %v", err)
	}
	defer pool.Close()

	detector := strategy.NewDetector(strategy.Config{
		Symbol:    *symbol,
		Window:    *window,
		Threshold: threshold,
		Cooldown:  *cooldown,
	})

	signals, tickCount, err := replay(ctx, pool, *exchange, *symbol, from, to, detector)
	if err != nil {
		log.Fatalf("replay: %v", err)
	}

	switch *format {
	case "csv":
		emitCSV(os.Stdout, signals)
	case "json":
		emitJSON(os.Stdout, signals)
	default:
		emitSummary(os.Stdout, *symbol, threshold, *window, *cooldown, from, to, tickCount, signals)
	}
}

// replay streams price ticks in [from, to] through the detector. Streaming
// avoids loading multi-day backtests into memory.
func replay(
	ctx context.Context,
	pool *pgxpool.Pool,
	exchange, symbol string,
	from, to time.Time,
	detector *strategy.Detector,
) ([]strategy.Signal, int64, error) {
	rows, err := pool.Query(ctx, `
		SELECT ts_exchange, price FROM price_ticks
		WHERE exchange = $1 AND symbol = $2 AND ts_exchange BETWEEN $3 AND $4
		ORDER BY ts_exchange ASC`, exchange, symbol, from, to)
	if err != nil {
		return nil, 0, fmt.Errorf("query: %w", err)
	}
	defer rows.Close()

	var (
		signals []strategy.Signal
		ticks   int64
	)
	for rows.Next() {
		var ts time.Time
		var price decimal.Decimal
		if err := rows.Scan(&ts, &price); err != nil {
			return nil, 0, fmt.Errorf("scan: %w", err)
		}
		ticks++
		if sig, ok := detector.OnTick(price, ts.UTC()); ok {
			signals = append(signals, *sig)
		}
	}
	return signals, ticks, rows.Err()
}

func emitSummary(
	w io.Writer,
	symbol string,
	threshold decimal.Decimal,
	window, cooldown time.Duration,
	from, to time.Time,
	ticks int64,
	signals []strategy.Signal,
) {
	_, _ = fmt.Fprintf(w, "Backtest: %s\n", symbol)
	_, _ = fmt.Fprintf(w, "Range:    %s → %s (%s)\n", from.Format(time.RFC3339), to.Format(time.RFC3339), to.Sub(from))
	_, _ = fmt.Fprintf(w, "Params:   threshold=%s window=%s cooldown=%s\n", threshold, window, cooldown)
	_, _ = fmt.Fprintf(w, "Ticks:    %s\n", commaInt(ticks))
	up, down := countDirections(signals)
	_, _ = fmt.Fprintf(w, "Signals:  %d (up=%d, down=%d)\n", len(signals), up, down)

	if ticks > 0 && len(signals) > 0 {
		rate := float64(len(signals)) / float64(ticks) * 100
		_, _ = fmt.Fprintf(w, "Rate:     %.3f%% of ticks emit a signal\n", rate)
	}
	if len(signals) == 0 {
		return
	}

	_, _ = fmt.Fprintln(w, "\nConfidence distribution:")
	for i, n := range bucketConfidence(signals) {
		_, _ = fmt.Fprintf(w, "  %.1f-%.1f  %s  %d\n", float64(i)/10, float64(i+1)/10, bar(n, len(signals)), n)
	}

	_, _ = fmt.Fprintln(w, "\nMagnitude stats:")
	min, avg, p50, p95, max := magnitudeStats(signals)
	_, _ = fmt.Fprintf(w, "  min=%s  avg=%s  p50=%s  p95=%s  max=%s\n", min, avg, p50, p95, max)

	n := 5
	if len(signals) < n {
		n = len(signals)
	}
	_, _ = fmt.Fprintln(w, "\nFirst signals:")
	for _, s := range signals[:n] {
		_, _ = fmt.Fprintf(w, "  %s  %-4s  mag=%-10s conf=%-6s windowMs=%d\n",
			s.DetectedAt.UTC().Format("15:04:05"), s.Direction,
			s.Magnitude.StringFixed(6), s.Confidence.StringFixed(3), s.WindowMs)
	}
}

func emitCSV(w io.Writer, signals []strategy.Signal) {
	cw := csv.NewWriter(w)
	defer cw.Flush()
	_ = cw.Write([]string{
		"detected_at", "direction", "magnitude", "window_ms", "confidence",
		"binance_price", "window_start_price", "velocity_bps_per_sec",
	})
	for _, s := range signals {
		_ = cw.Write([]string{
			s.DetectedAt.UTC().Format(time.RFC3339Nano),
			string(s.Direction),
			s.Magnitude.String(),
			strconv.Itoa(s.WindowMs),
			s.Confidence.String(),
			s.Context.BinancePrice.String(),
			s.Context.WindowStartPrice.String(),
			s.Context.VelocityBpsPerSec.String(),
		})
	}
}

func emitJSON(w io.Writer, signals []strategy.Signal) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(signals)
}

func countDirections(signals []strategy.Signal) (up, down int) {
	for _, s := range signals {
		switch s.Direction {
		case strategy.DirectionUp:
			up++
		case strategy.DirectionDown:
			down++
		}
	}
	return
}

func bucketConfidence(signals []strategy.Signal) []int {
	buckets := make([]int, 10)
	for _, s := range signals {
		c, _ := s.Confidence.Float64()
		idx := int(c * 10)
		if idx > 9 {
			idx = 9
		}
		if idx < 0 {
			idx = 0
		}
		buckets[idx]++ //nolint:gosec // idx clamped to [0,9] above

	}
	return buckets
}

func magnitudeStats(signals []strategy.Signal) (min, avg, p50, p95, max string) {
	floats := make([]float64, 0, len(signals))
	for _, s := range signals {
		f, _ := s.Magnitude.Float64()
		floats = append(floats, f)
	}
	sort.Float64s(floats)
	var sum float64
	for _, f := range floats {
		sum += f
	}
	avgF := sum / float64(len(floats))
	pick := func(p float64) float64 {
		idx := int(float64(len(floats)-1) * p)
		return floats[idx]
	}
	return fmt6(floats[0]), fmt6(avgF), fmt6(pick(0.5)), fmt6(pick(0.95)), fmt6(floats[len(floats)-1])
}

func fmt6(f float64) string {
	return strconv.FormatFloat(f, 'f', 6, 64)
}

func bar(n, total int) string {
	width := 30
	if total == 0 {
		return ""
	}
	filled := int(float64(n) / float64(total) * float64(width))
	if filled > width {
		filled = width
	}
	out := make([]rune, width)
	for i := 0; i < width; i++ {
		if i < filled {
			out[i] = '█'
		} else {
			out[i] = '·'
		}
	}
	return string(out)
}

func commaInt(n int64) string {
	s := strconv.FormatInt(n, 10)
	if len(s) <= 3 {
		return s
	}
	out := make([]byte, 0, len(s)+len(s)/3)
	for i, c := range []byte(s) {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, c)
	}
	return string(out)
}
