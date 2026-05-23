---
description: Audit code changes for trading risk issues
---

Review the current branch/working changes against HANDOFF.md Section 6 (Critical Implementation Rules).

Check specifically:
- [ ] No `float64` for money, prices, or sizes (must use Decimal)
- [ ] All timestamps are UTC
- [ ] WebSocket has reconnect with exponential backoff
- [ ] Orders have client-generated idempotency keys
- [ ] Risk limits cannot be disabled via config (only via code change)
- [ ] No secrets in logs
- [ ] Graceful shutdown handles in-flight orders
- [ ] Test coverage on strategy/risk modules is 100%

Report findings as:
- 🔴 CRITICAL: must fix before merge
- 🟡 WARNING: should fix
- 🟢 OK: verified compliant

Do not approve if any 🔴 exists.