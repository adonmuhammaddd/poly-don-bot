---
name: polymarket-api
description: Use when implementing Polymarket CLOB API integration. Triggers on changes to internal/feeds/polymarket/ or internal/execution/live.go, or when user mentions "Polymarket", "CLOB", "L1 auth", "L2 auth", or "order signing". Encodes Polymarket-specific gotchas that aren't in their official docs.
---

# Polymarket CLOB Integration Notes

## Auth Layers
- **L1 (wallet sig):** Sign with Polygon private key. Used for: creating L2 creds, withdrawing.
- **L2 (HMAC):** Used for: all trading operations.

Store L2 creds in env vars, never in code, never in logs.

## Order Submission Gotchas

1. **Market orders are FOK by default.** Use IOC if partial fill acceptable.
2. **`tokenID` ≠ `marketID`.** Each market has 2 token IDs (YES and NO). Cache the mapping.
3. **Price must be in cents.** $0.52 → send `52`. Decimal in API responses, integer in requests.
4. **Order size minimum: $1.** Below this rejected silently in some endpoints.
5. **Signature expires in 5 minutes.** Don't pre-sign and queue.

## WebSocket Gotchas

1. **Subscribe AFTER connection confirmed.** Sending subscribe before `auth` ack = silent drop.
2. **`book` channel = full snapshot.** `price_change` channel = delta. Use both, reconcile.
3. **Reconnect = re-subscribe.** Server doesn't remember.

## Rate Limits (verify on each implementation)
- Public: 100 req/min per IP
- Trading: 60 orders/min per API key
- WebSocket: connection-level, unclear documented

## What to Do When Things Fail
- 401: L2 creds expired, regenerate via L1
- 429: rate limited, exponential backoff with jitter
- 5xx: retry idempotent ops with same client_order_id
- Order rejected: log full response, do NOT auto-retry (could be valid rejection)

## References
- Official docs: https://docs.polymarket.com/developers/CLOB
- TypeScript reference impl: https://github.com/Polymarket/clob-client (read for behavior, port to Go)