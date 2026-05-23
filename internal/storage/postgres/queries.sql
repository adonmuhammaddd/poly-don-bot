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
