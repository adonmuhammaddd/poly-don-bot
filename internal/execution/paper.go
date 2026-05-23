package execution

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/shopspring/decimal"

	"github.com/adonmuhammaddd/poly-don-bot/internal/observability"
	"github.com/adonmuhammaddd/poly-don-bot/internal/risk"
	"github.com/adonmuhammaddd/poly-don-bot/internal/storage/postgres"
	"github.com/adonmuhammaddd/poly-don-bot/internal/strategy"
)

// PaperExecutor turns approved signals into paper Trade rows. It implements
// strategy.SignalListener and is subscribed alongside signals.Persister.
type PaperExecutor struct {
	cfg     PaperConfig
	storage Storage
	risk    RiskGate
	logger  *slog.Logger
	metrics *observability.Metrics

	dbTimeout time.Duration
}

func NewPaperExecutor(cfg PaperConfig, storage Storage, riskGate RiskGate, metrics *observability.Metrics, logger *slog.Logger) *PaperExecutor {
	if cfg.StartingBalanceUSD.IsZero() {
		cfg.StartingBalanceUSD = decimal.NewFromInt(100)
	}
	if cfg.SlippageTick.IsZero() {
		cfg.SlippageTick = decimal.NewFromFloat(0.01)
	}
	if cfg.MaxFillPrice.IsZero() {
		cfg.MaxFillPrice = decimal.NewFromFloat(0.99)
	}
	if cfg.MarketLookbackTime <= 0 {
		cfg.MarketLookbackTime = 5 * time.Minute
	}
	return &PaperExecutor{
		cfg:       cfg,
		storage:   storage,
		risk:      riskGate,
		logger:    logger.With(slog.String("component", "paper_executor")),
		metrics:   metrics,
		dbTimeout: 2 * time.Second,
	}
}

// OnSignal implements strategy.SignalListener.
func (p *PaperExecutor) OnSignal(sig *strategy.Signal) {
	ctx, cancel := context.WithTimeout(context.Background(), p.dbTimeout)
	defer cancel()

	now := time.Now().UTC()

	acct, err := p.accountState(ctx, now)
	if err != nil {
		p.metrics.PaperTradesSkipped.WithLabelValues("account_state_err").Inc()
		p.logger.Error("account state", slog.Any("error", err))
		return
	}

	decision := p.risk.Decide(sig, acct)
	if !decision.Approved {
		p.metrics.PaperTradesSkipped.WithLabelValues(decision.Reason).Inc()
		p.logger.Info("signal rejected by risk", slog.String("reason", decision.Reason))
		return
	}

	market, err := p.storage.GetLatestActiveMarket(ctx, pgtype.Timestamptz{Time: now.Add(-p.cfg.MarketLookbackTime), Valid: true})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			p.metrics.PaperTradesSkipped.WithLabelValues(SkipReasonNoMarket).Inc()
			p.logger.Info("no active market for signal", slog.String("direction", string(sig.Direction)))
			return
		}
		p.metrics.PaperTradesSkipped.WithLabelValues("market_lookup_err").Inc()
		p.logger.Error("market lookup", slog.Any("error", err))
		return
	}

	side, fillPrice, ok := p.computeFill(ctx, market.MarketID, sig.Direction)
	if !ok {
		return
	}

	params := postgres.InsertTradeParams{
		SignalID:   pgtype.Int8{},
		Mode:       ModePaper,
		MarketID:   market.MarketID,
		Side:       side,
		EntryPrice: fillPrice,
		SizeUsd:    decision.PositionSizeUSD,
		OpenedAt:   pgtype.Timestamptz{Time: now, Valid: true},
		Status:     StatusOpen,
	}
	id, err := p.storage.InsertTrade(ctx, params)
	if err != nil {
		p.metrics.PaperTradesSkipped.WithLabelValues(SkipReasonInsertFail).Inc()
		p.logger.Error("insert paper trade", slog.Any("error", err))
		return
	}

	p.metrics.PaperTradesOpened.WithLabelValues(side).Inc()
	p.logger.Info("paper trade opened",
		slog.Int64("id", id),
		slog.String("side", side),
		slog.String("entry_price", fillPrice.String()),
		slog.String("size_usd", decision.PositionSizeUSD.String()),
		slog.String("question", market.Question),
	)
}

func (p *PaperExecutor) computeFill(ctx context.Context, marketID string, dir strategy.Direction) (string, decimal.Decimal, bool) {
	if dir == strategy.DirectionUp {
		yes, err := p.storage.GetLatestYesQuote(ctx, marketID)
		if err != nil {
			p.metrics.PaperTradesSkipped.WithLabelValues(SkipReasonNoQuote).Inc()
			return "", decimal.Zero, false
		}
		if !yes.YesAsk.Valid {
			p.metrics.PaperTradesSkipped.WithLabelValues(SkipReasonZeroAsk).Inc()
			return "", decimal.Zero, false
		}
		fill := yes.YesAsk.Decimal.Add(p.cfg.SlippageTick)
		if fill.GreaterThan(p.cfg.MaxFillPrice) {
			p.metrics.PaperTradesSkipped.WithLabelValues(SkipReasonPriceTooHigh).Inc()
			return "", decimal.Zero, false
		}
		return SideYES, fill, true
	}

	no, err := p.storage.GetLatestNoQuote(ctx, marketID)
	if err != nil {
		p.metrics.PaperTradesSkipped.WithLabelValues(SkipReasonNoQuote).Inc()
		return "", decimal.Zero, false
	}
	if !no.NoAsk.Valid {
		p.metrics.PaperTradesSkipped.WithLabelValues(SkipReasonZeroAsk).Inc()
		return "", decimal.Zero, false
	}
	fill := no.NoAsk.Decimal.Add(p.cfg.SlippageTick)
	if fill.GreaterThan(p.cfg.MaxFillPrice) {
		p.metrics.PaperTradesSkipped.WithLabelValues(SkipReasonPriceTooHigh).Inc()
		return "", decimal.Zero, false
	}
	return SideNO, fill, true
}

// accountState computes the live snapshot risk needs from the trades table.
// Phase 3 PR 11 assumes no closed trades yet; PR 12 adds realized PnL.
func (p *PaperExecutor) accountState(ctx context.Context, now time.Time) (risk.AccountState, error) {
	openSizes, err := p.storage.ListOpenPaperTradeSizes(ctx)
	if err != nil {
		return risk.AccountState{}, err
	}
	committed := decimal.Zero
	for _, s := range openSizes {
		committed = committed.Add(s)
	}

	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	todayTrades, err := p.storage.CountTodayPaperTrades(ctx, pgtype.Timestamptz{Time: startOfDay, Valid: true})
	if err != nil {
		return risk.AccountState{}, err
	}

	return risk.AccountState{
		BalanceUSD:        p.cfg.StartingBalanceUSD.Sub(committed),
		StartOfDayBalance: p.cfg.StartingBalanceUSD,
		TodayPnLUSD:       decimal.Zero,
		TodayTradesCount:  int(todayTrades),
		ConsecutiveLosses: 0,
		Now:               now,
	}, nil
}
