# Polymarket Latency Arbitrage Bot — Handoff Document

> **For Claude Code**: This document is the source of truth for building a Polymarket latency arbitrage trading bot. Read this entire document before writing any code. Do NOT start coding until the user confirms understanding and gives explicit go-ahead on Phase 1.

---

## 1. Context & Goal

### What we're building
A trading bot that exploits the latency between Binance (BTC/USDT spot price) and Polymarket's 5-minute BTC Up/Down prediction markets. When BTC moves sharply on Binance, Polymarket's order book lags 30–90 seconds before repricing. The bot detects this lag and takes positions on the correct side before odds adjust.

### What we're NOT building
- A market predictor / ML price forecaster
- A high-frequency market maker
- A get-rich-quick scheme
- Anything that requires sub-millisecond execution

### Success criteria (Phase 1)
- Paper trading mode that ingests live WebSocket data from Binance + Polymarket
- Detects momentum signals reliably
- Logs would-be trades with full context (entry price, signal confidence, timestamp)
- 100% test coverage on strategy and risk modules
- Zero real money risk

### Operator profile
- Full-stack TypeScript developer, 5+ years
- Backend: Node.js, NestJS, Laravel
- New to Go, new to crypto trading
- Has read full strategy/architecture context (see Section 11)

---

## 2. Tech Stack — Locked In

| Layer | Choice | Reason |
|---|---|---|
| Bot core (hot path) | **Go 1.22+** | GC pause <1ms, mature WebSocket libs, ecosystem matches Polymarket infra |
| Operations dashboard | **Next.js 14 (App Router) + TypeScript + Tailwind** | Operator's existing skill, fast to ship |
| Inter-service comms | **REST + Server-Sent Events** for live updates | Simpler than gRPC for solo dev, SSE = native browser support |
| Database | **PostgreSQL 16** | Trade history, signals, PnL audit trail |
| Cache / pub-sub | **Redis 7** | Rate limiting, position state, real-time updates |
| Observability | **Prometheus + Grafana** | Industry standard, free, self-hosted |
| Alerting | **Telegram bot** | Free, mobile push, easy to integrate |
| Deployment | **Docker Compose** (Phase 1), Kubernetes (later if scale demands) | Phase 1 = single VPS, no need for k8s complexity |
| VPS region | **Amsterdam or Frankfurt** (Hetzner CX22 minimum) | Closest to Polymarket infra |

### Hard constraints
- **Go for the bot core.** Do not propose Node.js or Python for hot path. Decision is final.
- **TypeScript strict mode** for dashboard. No `any`, no implicit returns.
- **No ORM in Go** — use `sqlc` for type-safe SQL. Reason: ORMs hide query cost, dangerous in trading systems.
- **No Polymarket SDK exists for Go.** We build our own client.

---

## 3. Phased Delivery Plan

**CRITICAL**: Do not skip phases. Each phase must be reviewed and approved by operator before moving to next. Do not propose "let me just also add X" — stay scoped.

### Phase 1 — Observation Infrastructure (Week 1–2)
**Goal:** See the market clearly. Zero trading.

Deliverables:
- Go service that connects to Binance BTC/USDT WebSocket
- Go service that connects to Polymarket CLOB WebSocket (BTC 5min markets)
- Both feeds log to PostgreSQL with timestamps
- Next.js dashboard showing live prices side-by-side
- Latency measurement: difference between Binance tick and Polymarket reprice
- Docker Compose setup running locally

**Done when:** Operator can watch live dashboard for 1 hour and see the lag visually during a BTC move.

### Phase 2 — Signal Detection (Week 3)
**Goal:** Detect momentum, log signals. Still zero trading.

Deliverables:
- Momentum detector module (configurable window, threshold)
- Signal logger to PostgreSQL with full context
- Dashboard panel: live signal feed with confidence scores
- Backtest harness: replay historical data, output signal stats
- Unit tests: 100% coverage on detector logic

**Done when:** 7 days of signal logs show clear pattern; operator can review signals and intuit whether they would have been profitable.

### Phase 3 — Paper Trading (Week 4–5)
**Goal:** Simulate fills, track simulated PnL.

Deliverables:
- Paper executor: simulates Polymarket order fills using real order book
- Position tracker (in-memory + Redis backup)
- PnL calculator with realistic fees (Polymarket 2%, gas estimates)
- Dashboard: simulated positions, PnL chart, win rate, Sharpe ratio
- Risk module: position sizing, daily loss limits, circuit breaker
- Telegram alerts on simulated trades

**Done when:** 14 days of paper trading shows positive expected value AFTER fees and realistic slippage assumptions.

### Phase 4 — Live Execution (Week 6+, ONLY IF Phase 3 PASSES)
**Goal:** Real money, small size, semi-auto mode.

Deliverables:
- Polymarket CLOB authenticated client (L1 wallet signing + L2 HMAC)
- Real executor with idempotency, retry logic
- Semi-auto mode: bot detects → Telegram approval button → bot executes
- Full audit log of every API call to Polymarket
- Kill switch: physical button in dashboard + Telegram command
- Reconciliation: compare bot's view of positions vs Polymarket API every 60s

**Done when:** 7 days live with $50 starting capital, no critical incidents, PnL within 30% of paper trading expectation.

### Phase 5 — Full Auto (Month 3+)
Not in scope of initial handoff. Operator will commission separately after Phase 4 success.

---

## 4. Architecture

### High-level diagram

```
┌──────────────────────────────────────────────────────────────────┐
│                    VPS (Amsterdam/Frankfurt)                      │
│                                                                   │
│  ┌────────────────────┐         ┌────────────────────┐           │
│  │  Bot Core (Go)     │         │  Dashboard (Next)  │           │
│  │                    │  REST   │                    │           │
│  │  ┌──────────────┐  │ ◄─────► │  - Live prices     │           │
│  │  │ Feeds        │  │  SSE    │  - Signal feed     │           │
│  │  │ - Binance WS │  │ ─────►  │  - Positions       │           │
│  │  │ - Polymarket │  │         │  - PnL charts      │           │
│  │  └──────┬───────┘  │         │  - Kill switch     │           │
│  │         ▼          │         └────────────────────┘           │
│  │  ┌──────────────┐  │                                          │
│  │  │ Strategy     │  │         ┌────────────────────┐           │
│  │  │ - Momentum   │  │         │  Telegram Bot      │           │
│  │  │ - Risk       │  │ ──────► │  - Alerts          │           │
│  │  └──────┬───────┘  │         │  - Approvals       │           │
│  │         ▼          │         │  - Kill commands   │           │
│  │  ┌──────────────┐  │         └────────────────────┘           │
│  │  │ Executor     │  │                                          │
│  │  │ - Paper      │  │         ┌────────────────────┐           │
│  │  │ - Live       │  │ ──────► │  Polymarket CLOB   │           │
│  │  └──────┬───────┘  │   API   │  (Polygon)         │           │
│  │         ▼          │         └────────────────────┘           │
│  │  ┌──────────────┐  │                                          │
│  │  │ Storage      │  │         ┌────────────────────┐           │
│  │  │ - Postgres   │ ◄┼───────► │  PostgreSQL        │           │
│  │  │ - Redis      │ ◄┼───────► │  Redis             │           │
│  │  └──────────────┘  │         └────────────────────┘           │
│  │                    │                                          │
│  │  ┌──────────────┐  │         ┌────────────────────┐           │
│  │  │ Metrics      │ ─┼───────► │  Prometheus        │           │
│  │  └──────────────┘  │         │  + Grafana         │           │
│  └────────────────────┘         └────────────────────┘           │
└──────────────────────────────────────────────────────────────────┘
```

### Module boundaries (Go bot)

```
cmd/
└── bot/
    └── main.go              # Entry point, DI wiring, graceful shutdown

internal/
├── feeds/
│   ├── binance/             # Binance WebSocket client
│   │   ├── client.go
│   │   ├── client_test.go
│   │   └── types.go
│   └── polymarket/          # Polymarket CLOB WebSocket + REST
│       ├── ws_client.go
│       ├── rest_client.go
│       ├── auth.go          # L1 + L2 auth
│       └── types.go
├── strategy/
│   ├── momentum.go          # Signal detection
│   ├── momentum_test.go
│   └── types.go
├── risk/
│   ├── limits.go            # Position sizing, daily limits
│   ├── circuit_breaker.go
│   └── *_test.go
├── execution/
│   ├── executor.go          # Interface
│   ├── paper.go             # Paper trading impl
│   ├── live.go              # Real Polymarket impl
│   └── *_test.go
├── storage/
│   ├── postgres/
│   │   ├── queries.sql      # sqlc input
│   │   ├── queries.sql.go   # sqlc generated
│   │   └── migrations/
│   └── redis/
│       └── client.go
├── api/                     # REST + SSE for dashboard
│   ├── server.go
│   ├── handlers/
│   └── middleware/
├── alerts/
│   └── telegram/
└── observability/
    ├── metrics.go           # Prometheus
    └── logger.go            # Structured logging (slog)

pkg/                          # Reusable, can be imported by other tools
└── decimal/                  # Fixed-point math (NEVER use float for money)

migrations/                   # Database schema
configs/
└── config.example.yaml
```

### Dashboard structure (Next.js)

```
app/
├── (dashboard)/
│   ├── layout.tsx
│   ├── page.tsx                    # Overview: prices, PnL, signal feed
│   ├── positions/page.tsx          # Open + historical positions
│   ├── signals/page.tsx            # Signal log with filters
│   ├── settings/page.tsx           # Config tuning (read-only in Phase 1)
│   └── ops/
│       ├── page.tsx                # Kill switch, manual controls
│       └── logs/page.tsx           # Execution log viewer
├── api/                            # Proxy to Go backend (BFF pattern)
│   └── trpc/[trpc]/route.ts        # OR REST routes — pick one, not both
└── components/
    ├── price-ticker/
    ├── signal-feed/
    ├── pnl-chart/
    └── kill-switch/

lib/
├── api-client.ts                   # Typed client to Go backend
├── sse.ts                          # SSE hook
└── types.ts                        # Shared types (mirror Go types)
```

---

## 5. Data Models

### PostgreSQL schema (Phase 1)

```sql
-- migrations/0001_initial.sql

CREATE TABLE price_ticks (
  id            BIGSERIAL PRIMARY KEY,
  exchange      TEXT NOT NULL,
  symbol        TEXT NOT NULL,
  price         NUMERIC(20, 8) NOT NULL,
  ts_exchange   TIMESTAMPTZ NOT NULL,  -- timestamp from exchange
  ts_received   TIMESTAMPTZ NOT NULL DEFAULT NOW(),  -- when we got it
  raw_payload   JSONB
);

CREATE INDEX idx_price_ticks_lookup ON price_ticks (exchange, symbol, ts_exchange DESC);

CREATE TABLE polymarket_books (
  id            BIGSERIAL PRIMARY KEY,
  market_id     TEXT NOT NULL,
  question      TEXT NOT NULL,
  yes_bid       NUMERIC(8, 6),
  yes_ask       NUMERIC(8, 6),
  no_bid        NUMERIC(8, 6),
  no_ask        NUMERIC(8, 6),
  ts_received   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  raw_payload   JSONB
);

CREATE INDEX idx_pm_books_market ON polymarket_books (market_id, ts_received DESC);

CREATE TABLE signals (
  id              BIGSERIAL PRIMARY KEY,
  symbol          TEXT NOT NULL,
  direction       TEXT NOT NULL CHECK (direction IN ('up', 'down')),
  magnitude       NUMERIC(10, 6) NOT NULL,
  window_ms       INTEGER NOT NULL,
  confidence      NUMERIC(4, 3) NOT NULL,
  detected_at     TIMESTAMPTZ NOT NULL,
  context         JSONB NOT NULL,  -- snapshot of feeds at detection time
  action_taken    TEXT,            -- 'skipped' | 'paper_executed' | 'live_executed'
  action_reason   TEXT             -- why skipped/executed
);

CREATE INDEX idx_signals_time ON signals (detected_at DESC);

CREATE TABLE trades (
  id              BIGSERIAL PRIMARY KEY,
  signal_id       BIGINT REFERENCES signals(id),
  mode            TEXT NOT NULL CHECK (mode IN ('paper', 'live')),
  market_id       TEXT NOT NULL,
  side            TEXT NOT NULL CHECK (side IN ('YES', 'NO')),
  entry_price     NUMERIC(8, 6) NOT NULL,
  size_usd        NUMERIC(12, 2) NOT NULL,
  exit_price      NUMERIC(8, 6),
  pnl_usd         NUMERIC(12, 2),
  fees_usd        NUMERIC(12, 2),
  opened_at       TIMESTAMPTZ NOT NULL,
  closed_at       TIMESTAMPTZ,
  status          TEXT NOT NULL CHECK (status IN ('open', 'closed', 'failed')),
  external_order_id TEXT  -- Polymarket order ID for live mode
);

CREATE INDEX idx_trades_status ON trades (status, opened_at DESC);
CREATE INDEX idx_trades_mode ON trades (mode, opened_at DESC);
```

### Type contracts (shared)

```go
// internal/strategy/types.go
type PriceTick struct {
    Exchange   string    `json:"exchange"`
    Symbol     string    `json:"symbol"`
    Price      Decimal   `json:"price"`       // never float64
    Timestamp  time.Time `json:"timestamp"`
    ReceivedAt time.Time `json:"received_at"`
}

type Signal struct {
    ID           int64     `json:"id"`
    Symbol       string    `json:"symbol"`
    Direction    Direction `json:"direction"` // "up" | "down"
    Magnitude    Decimal   `json:"magnitude"` // % move as decimal (0.0025 = 0.25%)
    WindowMs     int       `json:"window_ms"`
    Confidence   Decimal   `json:"confidence"` // 0..1
    DetectedAt   time.Time `json:"detected_at"`
    Context      Context   `json:"context"`
}
```

```typescript
// dashboard/lib/types.ts — MUST mirror Go types exactly
export type Direction = 'up' | 'down';

export interface PriceTick {
  exchange: string;
  symbol: string;
  price: string;        // decimal as string (preserve precision)
  timestamp: string;    // ISO 8601
  receivedAt: string;
}

export interface Signal {
  id: number;
  symbol: string;
  direction: Direction;
  magnitude: string;
  windowMs: number;
  confidence: string;
  detectedAt: string;
  context: SignalContext;
}
```

---

## 6. Critical Implementation Rules

### Money / decimal handling
**NEVER use `float64` for price, size, or PnL.** Use `shopspring/decimal` library or wrap in custom `Decimal` type. Float arithmetic introduces rounding errors that accumulate across thousands of trades.

```go
// ❌ WRONG
var price float64 = 0.52
var size float64 = 100.0
var cost = price * size  // bug magnet

// ✅ RIGHT
price := decimal.NewFromString("0.52")
size := decimal.NewFromInt(100)
cost := price.Mul(size)
```

### Timestamps
- Always store UTC in PostgreSQL (`TIMESTAMPTZ`).
- Always use `time.Time` in Go with explicit UTC: `time.Now().UTC()`.
- Always use ISO 8601 over the wire.
- Track BOTH exchange timestamp AND received timestamp. Difference = useful latency metric.

### WebSocket reliability
- Exponential backoff on reconnect (1s, 2s, 4s, 8s, max 30s).
- Heartbeat / ping every 30s. If no pong in 10s, force reconnect.
- On reconnect, fetch REST snapshot to fill gap.
- Log every disconnect with reason. Disconnect rate >5/hour = alert.

### Idempotency
Every order must have a client-generated unique ID. On retry after timeout, reuse same ID. Polymarket API supports this — use it.

### Risk limits — non-negotiable defaults

```yaml
# configs/risk.yaml — values for Phase 4 live mode
max_position_size_usd: 50          # absolute cap per trade
max_position_pct_balance: 0.05     # 5% of balance per trade
max_trades_per_day: 30
max_daily_loss_usd: 30             # auto-stop if reached
max_daily_loss_pct: 0.15           # 15% drawdown auto-stop
max_consecutive_losses: 5          # pause 1 hour
max_signal_age_ms: 500             # drop signals older than this
min_balance_usd: 50                # stop trading below this
require_price_consensus: false     # Phase 4 = single feed (Binance), enable in Phase 5
```

These values are tunable in config, BUT the code must enforce that limits can NEVER be disabled. Removing a limit requires code change, not config change.

### Graceful shutdown
SIGINT/SIGTERM handler must:
1. Stop accepting new signals
2. Wait for in-flight orders to complete (max 30s)
3. Persist position state to Redis
4. Close DB connections
5. Send Telegram alert: "Bot shutdown — N open positions persisted"

### Logging
- Use `slog` (Go stdlib structured logging).
- JSON output in production, text in dev.
- NEVER log API secrets, wallet private keys, or full order signatures.
- Log levels: DEBUG (dev only), INFO (default), WARN (recoverable issues), ERROR (needs attention), FATAL (shutdown).

### Testing
- `internal/strategy/` and `internal/risk/` MUST have 100% test coverage with table-driven tests.
- `internal/execution/paper.go` MUST have integration tests with mocked order book.
- `internal/execution/live.go` MUST have a `--dry-run` flag that signs orders but doesn't submit them.

---

## 7. Polymarket CLOB Integration — Specifics

### Authentication (two-tier)

**L1 — Wallet signature:** Sign messages with Polygon wallet private key. Use `go-ethereum`'s `crypto.Sign`.

**L2 — API credentials:** Derived from L1 signature. Returns `apiKey`, `secret`, `passphrase`. Sign every request with HMAC-SHA256.

Authentication header format:
```
POLY_ADDRESS: 0x...           # wallet address
POLY_SIGNATURE: ...           # HMAC of timestamp+method+path+body
POLY_TIMESTAMP: 1234567890    # unix seconds
POLY_API_KEY: ...
POLY_PASSPHRASE: ...
```

Reference: https://docs.polymarket.com/developers/CLOB/authentication

### Rate limits
- Public API: 100 req/min
- Trading API: 60 orders/min
- WebSocket: no documented hard limit, but reconnect floods get IP banned

Implement client-side rate limiter (token bucket) to stay safely under.

### Order types we use
- **Market orders** (FOK or IOC) — for taking liquidity fast on signal
- **Limit orders** — NOT used in Phase 1–4

### Market resolution
The "BTC Up/Down 5min" markets cycle every 5 minutes. Bot must:
- Query active markets every 30s
- Identify the current 5-min window market
- Cache market ID + tokenID for YES and NO
- Handle the cutoff: bot must NOT place orders in the last 30 seconds of a window (settlement risk)

### Fee model
- Polymarket taker fee: 2% on profitable trades only (as of 2026, verify in implementation)
- Polygon gas: ~$0.007 per transaction
- Build fee calculator into PnL — gross PnL means nothing.

---

## 8. Operator Workflow

How the operator (user) will run this:

### Daily flow (Phase 1–2)
1. Morning: open dashboard, check overnight feed health
2. Review signals from past 24h
3. Tune detection threshold if needed (config change → restart)
4. Continue

### Daily flow (Phase 3 paper)
1. Morning: review paper PnL, win rate, signal-to-trade conversion
2. Compare to backtest expectations
3. If divergence >20%, investigate

### Daily flow (Phase 4 live, semi-auto)
1. Telegram notification on signal → review context → tap APPROVE or SKIP
2. Bot executes within 5s of approval
3. Morning: review previous day's PnL, position reconciliation
4. Weekly: review Sharpe, win rate trend, slippage

### Emergency kill
3 channels, any one works:
1. Dashboard red button (web UI)
2. Telegram command `/kill`
3. SSH to VPS, `docker compose stop bot`

All three must result in: cancel open orders, close positions at market, send confirmation alert.

---

## 9. What NOT to Build (Yet)

Common scope creep traps. Do not build any of these in initial phases:

- ❌ Multi-exchange support beyond Binance (Coinbase, Bybit) — Phase 5+
- ❌ Multiple strategies in parallel — Phase 5+
- ❌ ML / prediction models — explicitly out of scope
- ❌ Backtest UI — CLI tool with CSV/JSON output is enough
- ❌ User authentication on dashboard — single operator, VPS firewall + Tailscale is enough
- ❌ Multi-user / multi-account support — single account only
- ❌ Mobile app — Telegram bot is the mobile interface
- ❌ Tax reporting — CSV export of trades is enough, operator handles taxes externally
- ❌ Strategy marketplace / sharing — never
- ❌ "Just in case" abstractions — YAGNI. Don't build interface for one implementation.

If during implementation you find yourself wanting to add any of the above, STOP and ask the operator first.

---

## 10. Deliverables Checklist (Phase 1)

Before declaring Phase 1 done, verify:

- [ ] `docker compose up` brings up: Go bot, Postgres, Redis, Next.js dashboard, Prometheus, Grafana
- [ ] `.env.example` documents every required env var
- [ ] `README.md` covers: setup, dev workflow, deployment to VPS, troubleshooting
- [ ] `make test` runs full test suite, all green
- [ ] `make lint` runs golangci-lint + eslint, zero warnings
- [ ] Binance WS connects, logs ticks to Postgres
- [ ] Polymarket WS connects, logs book updates to Postgres
- [ ] Dashboard shows live BTC price from Binance AND Polymarket implied price side-by-side
- [ ] Dashboard shows latency metric: ms between Binance move and Polymarket reprice
- [ ] Grafana dashboard exists with: feed uptime, tick rate, WS disconnects
- [ ] Operator has run it locally for 1 hour and confirmed the lag is visible

---

## 11. Background Context (Read First)

The operator and Claude (web chat) have had extensive discussion that led to this handoff. Key conclusions:

1. **The viral "$0.90 → $408K in 2 days" claim is misleading.** Real returns are more modest. Bot edge is real but degrading (arbitrage window shrunk from 12.3s in 2024 to 2.7s in Q1 2026).

2. **Strategy chosen: Polymarket latency arbitrage on BTC 5-min Up/Down markets.** Reasoning: highest realistic edge for solo developer, doesn't require sub-ms latency, capital-efficient.

3. **Language chosen: Go.** Reasoning: time-to-ship matters more than peak performance. Rust considered but rejected for Phase 1–4 due to learning curve cost. Hybrid Go+Rust may be revisited at Phase 5.

4. **Operating mode progression:**
   - Phase 1–2: observation only, no trading
   - Phase 3: paper trading
   - Phase 4: semi-auto live (Telegram approval per trade)
   - Phase 5: full auto (not in this handoff scope)

5. **Operator is new to trading.** Code must be defensive: prefer crash over wrong execution. Default to skip-trade over take-trade when uncertain. Risk limits enforced in code, not just config.

6. **Operator's strength is TypeScript/React.** Dashboard is the operator's domain — they will own UX iteration. Bot core is the harder part to get right; spend implementation budget there.

7. **The bot is one part of a longer learning journey.** Even if this exact strategy stops being profitable, the infrastructure (feeds, risk, execution, observability) is reusable for next strategies.

---

## 12. First Steps for Claude Code

When the operator runs this handoff:

1. **Acknowledge the handoff.** Confirm you've read all 11 sections. Do NOT start coding.

2. **Ask clarifying questions** about anything ambiguous. Likely topics:
   - Operator's VPS provider preference and budget
   - Whether to use Tailscale or just SSH key auth
   - Telegram bot setup (operator has token? needs help creating?)
   - PostgreSQL: local Docker vs managed (Supabase/Neon)
   - Preferred logging aggregator if any (or stdout is fine for Phase 1)

3. **Propose a Phase 1 sprint plan.** Break it into ~5 PRs/commits, each independently reviewable:
   - PR 1: Repo scaffolding, Docker Compose, CI
   - PR 2: Binance WS feed + Postgres persistence
   - PR 3: Polymarket WS feed + Postgres persistence
   - PR 4: REST API (Go) + dashboard skeleton (Next.js)
   - PR 5: Dashboard live view + latency metric

4. **Wait for operator approval on the plan** before writing any code.

5. **For each PR**, write the code, write the tests, write a short summary of what's in it and what's NOT in it. Run `make test` and `make lint` before declaring done.

6. **Never skip writing tests.** If a module is "too simple to test", it's also too simple to skip the test.

7. **When uncertain, ask.** Trading systems fail in subtle, expensive ways. Cost of asking < cost of guessing wrong.

---

## 13. Reference Material

The operator has already reviewed:

- Polymarket CLOB docs: https://docs.polymarket.com/developers/CLOB
- Reference open-source bot (Python, for strategy reference only): https://github.com/learningworship/polymarket-latency-bot
- Background reading: "Trading and Exchanges" by Larry Harris

External APIs needed:
- Binance WebSocket: `wss://stream.binance.com:9443/stream`
- Polymarket WS: `wss://ws-subscriptions-clob.polymarket.com/ws/`
- Polymarket REST: `https://clob.polymarket.com`
- Telegram Bot API: https://core.telegram.org/bots/api

---

## 14. Final Notes

This is a **learning project that handles real money**. The operator's goals, in priority order:

1. **Don't lose more than $200 across the entire learning journey.** Risk limits exist to enforce this.
2. **Learn how production trading systems are built.** Code quality, tests, observability matter even if strategy fails.
3. **Validate whether this specific strategy is profitable.** Honest answer (profitable / not / inconclusive) is the success outcome, regardless of direction.
4. **Build reusable infrastructure for future strategies.**

Profit is a possible outcome, not the goal. Build accordingly.

---

**End of handoff. Await operator instruction.**
