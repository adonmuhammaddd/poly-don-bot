-- price_ticks: every tick we receive from any exchange (Binance for Phase 1).
CREATE TABLE price_ticks (
  id            BIGSERIAL PRIMARY KEY,
  exchange      TEXT NOT NULL,
  symbol        TEXT NOT NULL,
  price         NUMERIC(20, 8) NOT NULL,
  ts_exchange   TIMESTAMPTZ NOT NULL,
  ts_received   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
  raw_payload   JSONB
);

CREATE INDEX idx_price_ticks_lookup ON price_ticks (exchange, symbol, ts_exchange DESC);

-- polymarket_books: order book snapshots for Polymarket BTC Up/Down 5-min markets.
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

-- signals: detected momentum signals (Phase 2+, table scaffolded now).
CREATE TABLE signals (
  id              BIGSERIAL PRIMARY KEY,
  symbol          TEXT NOT NULL,
  direction       TEXT NOT NULL CHECK (direction IN ('up', 'down')),
  magnitude       NUMERIC(10, 6) NOT NULL,
  window_ms       INTEGER NOT NULL,
  confidence      NUMERIC(4, 3) NOT NULL,
  detected_at     TIMESTAMPTZ NOT NULL,
  context         JSONB NOT NULL,
  action_taken    TEXT,
  action_reason   TEXT
);

CREATE INDEX idx_signals_time ON signals (detected_at DESC);

-- trades: paper + live trade history (Phase 3+, table scaffolded now).
CREATE TABLE trades (
  id                  BIGSERIAL PRIMARY KEY,
  signal_id           BIGINT REFERENCES signals(id),
  mode                TEXT NOT NULL CHECK (mode IN ('paper', 'live')),
  market_id           TEXT NOT NULL,
  side                TEXT NOT NULL CHECK (side IN ('YES', 'NO')),
  entry_price         NUMERIC(8, 6) NOT NULL,
  size_usd            NUMERIC(12, 2) NOT NULL,
  exit_price          NUMERIC(8, 6),
  pnl_usd             NUMERIC(12, 2),
  fees_usd            NUMERIC(12, 2),
  opened_at           TIMESTAMPTZ NOT NULL,
  closed_at           TIMESTAMPTZ,
  status              TEXT NOT NULL CHECK (status IN ('open', 'closed', 'failed')),
  external_order_id   TEXT
);

CREATE INDEX idx_trades_status ON trades (status, opened_at DESC);
CREATE INDEX idx_trades_mode ON trades (mode, opened_at DESC);
