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
