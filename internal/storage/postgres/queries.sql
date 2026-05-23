-- name: InsertPriceTick :one
INSERT INTO price_ticks (
  exchange, symbol, price, ts_exchange, raw_payload
) VALUES (
  $1, $2, $3, $4, $5
)
RETURNING id;

-- name: GetLatestPriceTick :one
SELECT id, exchange, symbol, price, ts_exchange, ts_received
FROM price_ticks
WHERE exchange = $1 AND symbol = $2
ORDER BY ts_exchange DESC
LIMIT 1;

-- name: CountPriceTicksSince :one
SELECT count(*) FROM price_ticks
WHERE exchange = $1 AND ts_received >= $2;

-- name: InsertPolymarketBook :one
INSERT INTO polymarket_books (
  market_id, question, yes_bid, yes_ask, no_bid, no_ask, raw_payload
) VALUES (
  $1, $2, $3, $4, $5, $6, $7
)
RETURNING id;

-- name: CountPolymarketBooksSince :one
SELECT count(*) FROM polymarket_books
WHERE market_id = $1 AND ts_received >= $2;

-- name: GetLatestActiveMarket :one
SELECT market_id, question, ts_received AS last_seen
FROM polymarket_books
WHERE ts_received >= $1
ORDER BY ts_received DESC
LIMIT 1;

-- name: GetLatestYesQuote :one
SELECT yes_bid, yes_ask, ts_received
FROM polymarket_books
WHERE market_id = $1 AND yes_bid IS NOT NULL
ORDER BY ts_received DESC
LIMIT 1;

-- name: GetLatestNoQuote :one
SELECT no_bid, no_ask, ts_received
FROM polymarket_books
WHERE market_id = $1 AND no_bid IS NOT NULL
ORDER BY ts_received DESC
LIMIT 1;

-- name: InsertSignal :one
INSERT INTO signals (
  symbol, direction, magnitude, window_ms, confidence, detected_at, context, action_taken, action_reason
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8, $9
)
RETURNING id;

-- name: ListRecentSignals :many
SELECT id, symbol, direction, magnitude, window_ms, confidence, detected_at, context, action_taken, action_reason
FROM signals
ORDER BY detected_at DESC
LIMIT $1;

-- name: ListPriceTicksRange :many
SELECT ts_exchange, price
FROM price_ticks
WHERE exchange = sqlc.arg(exchange)
  AND symbol = sqlc.arg(symbol)
  AND ts_exchange BETWEEN sqlc.arg(from_ts) AND sqlc.arg(to_ts)
ORDER BY ts_exchange ASC;

-- name: InsertTrade :one
INSERT INTO trades (
  signal_id, mode, market_id, side, entry_price, size_usd, opened_at, status
) VALUES (
  $1, $2, $3, $4, $5, $6, $7, $8
)
RETURNING id;

-- name: ListOpenPaperTradeSizes :many
SELECT size_usd FROM trades
WHERE mode = 'paper' AND status = 'open';

-- name: CountTodayPaperTrades :one
SELECT count(*) FROM trades
WHERE mode = 'paper' AND opened_at >= $1;
