.DEFAULT_GOAL := help

# Auto-load .env if present so every target inherits POSTGRES_URL etc. without
# the caller having to `export` manually. `-include` is non-fatal if missing.
-include .env
export

POSTGRES_URL ?= postgres://poly:poly_dev_password@localhost:5432/polybot?sslmode=disable

.PHONY: help
help: ## Show this help
	@grep -hE '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-22s\033[0m %s\n", $$1, $$2}'

# ---- Go bot ----

.PHONY: dev
dev: ## Run bot locally with race detector
	go run -race ./cmd/bot

.PHONY: test
test: ## Run all Go tests
	go test -race -count=1 ./...

.PHONY: test-cover
test-cover: ## Run tests with coverage report (HTML at coverage.html)
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

.PHONY: lint
lint: ## Run golangci-lint
	golangci-lint run ./...

.PHONY: fmt
fmt: ## Format Go code
	go fmt ./...

.PHONY: build
build: ## Build bot binary into bin/bot
	go build -o bin/bot ./cmd/bot

.PHONY: tidy
tidy: ## Tidy go.mod
	go mod tidy

# ---- Migrations (uses golang-migrate CLI) ----

.PHONY: migrate-up
migrate-up: ## Apply pending migrations
	migrate -path migrations -database "$(POSTGRES_URL)" up

.PHONY: migrate-down
migrate-down: ## Roll back last migration
	migrate -path migrations -database "$(POSTGRES_URL)" down 1

.PHONY: migrate-create
migrate-create: ## Create new migration: make migrate-create name=add_foo
	@if [ -z "$(name)" ]; then echo "Usage: make migrate-create name=<migration_name>"; exit 1; fi
	migrate create -ext sql -dir migrations -seq $(name)

.PHONY: migrate-version
migrate-version: ## Show current migration version
	migrate -path migrations -database "$(POSTGRES_URL)" version

# ---- sqlc (wired in PR 2 when queries.sql exists) ----

.PHONY: sqlc
sqlc: ## Generate type-safe Go from queries.sql
	sqlc generate

# ---- Dashboard ----

.PHONY: dashboard-install
dashboard-install: ## Install dashboard deps
	cd dashboard && pnpm install

.PHONY: dashboard-dev
dashboard-dev: ## Run Next.js dashboard in dev mode
	cd dashboard && pnpm dev

.PHONY: dashboard-lint
dashboard-lint: ## Lint dashboard
	cd dashboard && pnpm lint

.PHONY: dashboard-typecheck
dashboard-typecheck: ## Typecheck dashboard
	cd dashboard && pnpm typecheck

.PHONY: dashboard-build
dashboard-build: ## Build dashboard for production
	cd dashboard && pnpm build

# ---- Docker infrastructure ----

.PHONY: up
up: ## Start infrastructure (postgres, redis, prometheus, grafana)
	docker compose up -d postgres redis prometheus grafana

.PHONY: down
down: ## Stop all services (preserves volumes)
	docker compose down

.PHONY: logs
logs: ## Tail logs from infrastructure services
	docker compose logs -f postgres redis prometheus grafana

.PHONY: clean
clean: ## Stop services + remove volumes (WARNING: deletes Postgres/Redis data)
	docker compose down -v

# ---- First-time setup ----

.PHONY: ensure-env
ensure-env: ## Create .env from .env.example if missing
	@if [ ! -f .env ]; then cp .env.example .env && echo ".env created from .env.example"; fi

.PHONY: setup
setup: ensure-env up ## First-time setup: create .env + start infra + wait + run migrations
	@echo "Waiting for postgres..."
	@until docker exec polybot-postgres pg_isready -U poly -d polybot >/dev/null 2>&1; do sleep 1; done
	@echo "Postgres ready. Running migrations..."
	$(MAKE) migrate-up
	@echo ""
	@echo "Setup complete."
	@echo "  Bot:        make dev"
	@echo "  Dashboard:  make dashboard-dev"
	@echo "  Grafana:    http://localhost:3001 (admin/admin)"
	@echo "  Prometheus: http://localhost:9090"
