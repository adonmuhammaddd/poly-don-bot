package signals

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/adonmuhammaddd/poly-don-bot/internal/observability"
	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
	"github.com/adonmuhammaddd/poly-don-bot/internal/strategy"
)

// Storage is the slice of *postgres.Queries that Persister uses.
type Storage interface {
	InsertSignal(ctx context.Context, arg postgres.InsertSignalParams) (int64, error)
	GetLatestActiveMarket(ctx context.Context, since pgtype.Timestamptz) (postgres.GetLatestActiveMarketRow, error)
	GetLatestYesQuote(ctx context.Context, marketID string) (postgres.GetLatestYesQuoteRow, error)
}

// Persister snapshots Polymarket context and writes a signal to Postgres.
// It implements strategy.SignalListener — feed pipeline detection lives in
// strategy.Runner, which fans out to this and other listeners.
type Persister struct {
	storage Storage
	logger  *slog.Logger
	metrics *observability.Metrics

	dbTimeout time.Duration
	actionTag string
}

func NewPersister(storage Storage, metrics *observability.Metrics, logger *slog.Logger) *Persister {
	return &Persister{
		storage:   storage,
		logger:    logger.With(slog.String("component", "signals")),
		metrics:   metrics,
		dbTimeout: 2 * time.Second,
		actionTag: "skipped",
	}
}

// OnSignal implements strategy.SignalListener. Snapshots the latest Polymarket
// market + YES quote, then writes the signal row.
func (p *Persister) OnSignal(sig *strategy.Signal) {
	p.metrics.SignalsDetected.WithLabelValues(string(sig.Direction)).Inc()

	ctx, cancel := context.WithTimeout(context.Background(), p.dbTimeout)
	defer cancel()

	pm := p.snapshotPolymarket(ctx)
	payload, err := json.Marshal(signalContext{
		Strategy:   sig.Context,
		Polymarket: pm,
	})
	if err != nil {
		p.metrics.SignalsPersistError.WithLabelValues("marshal").Inc()
		p.logger.Error("marshal signal context", slog.Any("error", err))
		return
	}

	params := postgres.InsertSignalParams{
		Symbol:    sig.Symbol,
		Direction: string(sig.Direction),
		Magnitude: sig.Magnitude.Round(6),
		WindowMs:  int32(sig.WindowMs), //nolint:gosec // detector window << int32 max

		Confidence:   sig.Confidence.Round(3),
		DetectedAt:   pgtype.Timestamptz{Time: sig.DetectedAt.UTC(), Valid: true},
		Context:      payload,
		ActionTaken:  pgtype.Text{String: p.actionTag, Valid: true},
		ActionReason: pgtype.Text{String: "phase_2_observation", Valid: true},
	}

	if _, err := p.storage.InsertSignal(ctx, params); err != nil {
		p.metrics.SignalsPersistError.WithLabelValues("insert").Inc()
		p.logger.Error("insert signal", slog.Any("error", err))
		return
	}

	p.logger.Info("signal persisted",
		slog.String("direction", string(sig.Direction)),
		slog.String("magnitude", sig.Magnitude.String()),
		slog.String("confidence", sig.Confidence.String()),
	)
}

// signalContext is the JSONB payload we store in signals.context.
type signalContext struct {
	Strategy   strategy.Context    `json:"strategy"`
	Polymarket *polymarketSnapshot `json:"polymarket,omitempty"`
}

type polymarketSnapshot struct {
	MarketID string  `json:"marketId"`
	Question string  `json:"question"`
	YesBid   *string `json:"yesBid,omitempty"`
	YesAsk   *string `json:"yesAsk,omitempty"`
	YesMid   *string `json:"yesMid,omitempty"`
}

func (p *Persister) snapshotPolymarket(ctx context.Context) *polymarketSnapshot {
	since := pgtype.Timestamptz{Time: time.Now().UTC().Add(-5 * time.Minute), Valid: true}
	market, err := p.storage.GetLatestActiveMarket(ctx, since)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			p.logger.Warn("polymarket market lookup failed", slog.Any("error", err))
		}
		return nil
	}

	snap := &polymarketSnapshot{
		MarketID: market.MarketID,
		Question: market.Question,
	}
	yes, err := p.storage.GetLatestYesQuote(ctx, market.MarketID)
	if err != nil {
		if !errors.Is(err, pgx.ErrNoRows) {
			p.logger.Warn("polymarket yes quote lookup failed", slog.Any("error", err))
		}
		return snap
	}
	if yes.YesBid.Valid {
		v := yes.YesBid.Decimal.String()
		snap.YesBid = &v
	}
	if yes.YesAsk.Valid {
		v := yes.YesAsk.Decimal.String()
		snap.YesAsk = &v
	}
	if yes.YesBid.Valid && yes.YesAsk.Valid {
		mid := yes.YesBid.Decimal.Add(yes.YesAsk.Decimal).Div(decimal.NewFromInt(2)).StringFixed(4)
		snap.YesMid = &mid
	}
	return snap
}
