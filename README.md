# Poly Don Bot

Polymarket latency arbitrage bot. Go core + Next.js dashboard.

**Spec:** `HANDOFF.md` is the source of truth. Read it before contributing.

**Status:** Phase 1 — observation infrastructure. Zero trading.

---

## Prerequisites

- **Go** 1.22+ — `brew install go`
- **Node.js** 22+ and **pnpm** 10+ — `brew install node && corepack enable`
- **Docker** with Docker Compose v2 (Docker Desktop or Colima)
- **golang-migrate** CLI — `brew install golang-migrate`
- **golangci-lint** (optional, for `make lint`) — `brew install golangci-lint`
- **sqlc** (optional, used from PR 2) — `brew install sqlc`

---

## First-time setup

```bash
cp .env.example .env                   # adjust if needed
make setup                             # starts infra + applies migrations
cd dashboard && pnpm install && cd ..  # install dashboard deps
```

`make setup` brings up Postgres, Redis, Prometheus, Grafana via Docker Compose, then runs migrations.

Verify services:

| Service    | URL                              | Notes                          |
|------------|----------------------------------|--------------------------------|
| Postgres   | `localhost:5432`                 | user/pass from `.env`          |
| Redis      | `localhost:6379`                 |                                |
| Prometheus | http://localhost:9090            |                                |
| Grafana    | http://localhost:3001            | admin / admin                  |
| Bot        | http://localhost:8080 (PR 4+)    | not running until PR 4         |
| Dashboard  | http://localhost:3000            | `make dashboard-dev`           |

---

## Daily dev workflow

```bash
make up                  # start infra if not running
make dev                 # run bot with race detector
make dashboard-dev       # run Next.js dashboard (separate terminal)
make test                # full Go test suite
make lint                # golangci-lint
make dashboard-lint      # next lint
make dashboard-typecheck # tsc --noEmit
```

Stop infra without losing data: `make down`. Nuke volumes: `make clean` (deletes Postgres + Redis data).

---

## Common commands

```bash
make help                          # list all targets
make migrate-up                    # apply migrations
make migrate-down                  # roll back last migration
make migrate-create name=add_foo   # create new migration file pair
make test-cover                    # tests + HTML coverage report
make build                         # build bot binary into bin/bot
```

---

## Repo layout

```
poly-don-bot/
├── cmd/bot/                # Go entry point
├── internal/               # Go internal packages (added PR 2+)
├── pkg/                    # reusable Go packages
├── migrations/             # SQL migrations (golang-migrate)
├── configs/                # config templates + Prometheus + Grafana provisioning
├── dashboard/              # Next.js 14 dashboard
├── docker-compose.yml      # infra services
├── Makefile                # dev workflow
└── HANDOFF.md              # spec — source of truth
```

---

## Troubleshooting

**Docker daemon not running**
Start Docker Desktop or `colima start`. Verify with `docker ps`.

**`docker compose up` fails with "port already in use"**
Another service is on the port. `lsof -i :5432` to find culprit, or change the host port in `docker-compose.yml`.

**Migrations fail with `dial tcp [::1]:5432: connect: connection refused`**
Postgres not ready yet. Wait a few seconds or check `docker compose ps`.

**Grafana shows "Prometheus data source not found"**
Check `configs/grafana/provisioning/datasources/prometheus.yaml`. Restart with `docker compose restart grafana`.

**Dashboard pnpm install fails**
Make sure pnpm 10+ is active: `corepack enable && corepack prepare pnpm@latest --activate`.

**`go: command not found`**
`brew install go`, then restart shell.

---

## Phase 1 acceptance

Per `HANDOFF.md` Section 10, Phase 1 is done when:

```bash
# Terminal 1: infra + bot
make setup              # if first time; otherwise: make up
make dev

# Terminal 2: dashboard
make dashboard-dev

# Terminal 3: verify endpoints
curl -s localhost:8080/api/health
curl -s localhost:8080/api/prices/latest?exchange=binance\&symbol=btcusdt
curl -s localhost:8080/api/polymarket/current
curl -s localhost:8080/api/latency/recent
```

Open:
- Dashboard → http://localhost:3000 (Binance + Polymarket cards + Latency widget)
- Grafana → http://localhost:3001 (admin/admin), navigate to "Poly Don Bot" dashboard
- Prometheus → http://localhost:9090

Watch the dashboard for ~1 hour. Acceptance items:

- [x] `docker compose up` brings up postgres, redis, prometheus, grafana
- [x] `.env.example` documents required env vars
- [x] README covers setup, dev workflow, troubleshooting
- [x] `make test` green
- [x] `make lint` zero warnings
- [x] Binance WS streams ticks into `price_ticks` table
- [x] Polymarket WS streams book updates into `polymarket_books` table
- [x] Dashboard shows live BTC price + Polymarket implied probability side-by-side
- [x] Dashboard shows latency widget (last delta, p50, p95, bar chart of recent pairs)
- [x] Grafana dashboard shows feed uptime, tick rate, disconnect rate
- [ ] **Operator confirms lag visible during a BTC move** ← manual sign-off

When BTC moves >0.05%, the Binance card jumps; the latency widget shows the ms it takes Polymarket to reprice. Bars are green &lt;1s, amber &lt;5s, red beyond.

---

## Contributing

- Conventional commit messages, scoped per package: `feat(feeds/binance): ...`
- 100% test coverage required on `internal/strategy/` and `internal/risk/`
- See `HANDOFF.md` Section 6 for critical implementation rules (decimal vs float, timestamps, idempotency, etc.)
- Never use `float64` for money. Use the decimal wrapper in `pkg/decimal/`.
