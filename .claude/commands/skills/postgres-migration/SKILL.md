---
name: postgres-migration
description: Use when creating or modifying PostgreSQL migrations in this repo. Triggers on file changes in migrations/ directory, or when user mentions "migration", "schema change", "ALTER TABLE", or "new table". Enforces migration safety rules for a trading system where downtime costs money.
---

# Migration Safety Rules

## Required

1. **Up + Down pair.** Every migration has reversible down. Test both.
2. **Numbered sequentially:** `NNNN_description.up.sql` and `NNNN_description.down.sql`
3. **Transactional by default.** Wrap in BEGIN/COMMIT unless creating indexes (use CONCURRENTLY).
4. **Backward compatible during deployment window.**
   - Add column NULL first, backfill, then NOT NULL in next migration
   - Never DROP COLUMN in same release as code that stops using it
5. **Trading data immutable.** Never UPDATE `trades` or `signals` table data in migration. Add new columns/tables instead.

## Process

Before writing migration:
1. Show the SQL
2. Show what data could be lost or corrupted
3. Show rollback plan
4. Wait for approval

After approval:
1. Write both up.sql and down.sql
2. Test against fresh DB: `make db-reset && make db-migrate`
3. Test rollback: `make db-rollback`
4. Update sqlc queries if schema changed
5. Regenerate: `make sqlc-generate`

## Anti-patterns

- `DROP TABLE` without confirmation
- Renaming column (use add new + copy + drop pattern across 2+ migrations)
- Adding NOT NULL column without default
- Index without CONCURRENTLY on table >10k rows