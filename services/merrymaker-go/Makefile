# Merrymaker Development Makefile
#
# Quick Start:
#   make dev-full         # [RECOMMENDED] Start dev DB + Air (full dev environment with live-reload)
#   make dev              # Start Air only (live-reload Go + frontend assets)
#
# Development:
#   make dev-db-up        # Start dev Postgres (profile dev)
#   make dev-db-down      # Stop dev Postgres (preserves data)
#   make dev-db-seed      # Seed dev database with sample data
#   make dev-db-verify    # Verify dev database has seeded data
#
# Testing:
#   make test             # Run tests against test DB (brings it up if needed)
#   make test-with-infra  # Run tests with full infrastructure (Postgres + Redis)
#   make test-workflows   # Run workflow tests only
#
# Linting:
#   make lint             # Run golangci-lint
#   make lint-fix         # Run golangci-lint with auto-fix
#
# Frontend:
#   make frontend-test    # Run frontend JavaScript tests
#   make frontend-lint    # Run frontend linter (Biome)
#   make frontend-build   # Build frontend assets

COMPOSE=docker compose
COMPOSE_FILE=docker-compose.yml

# Test DB connection env (used by internal/testutil)
TEST_DB_HOST?=localhost
TEST_DB_PORT?=55432
TEST_DB_USER?=merrymaker
TEST_DB_PASSWORD?=merrymaker
TEST_DB_NAME?=merrymaker


# Test selection controls
PKG?=./...
TESTARGS?=

.PHONY: dev dev-full dev-db-up dev-db-down dev-db-destroy dev-db-seed dev-db-verify test-db-up test-db-down wait-test-db test test-ephemeral

## Development with live-reload (Air)
dev:
	@echo "[INFO] Starting Air (live-reload for Go + frontend assets)..."
	@echo "[INFO] Edit any .go, .css, .js, .ts, or template file and see changes automatically"
	@echo "[INFO] Make sure you have Air installed: go install github.com/air-verse/air@v1.63.0"
	@command -v air >/dev/null 2>&1 || { echo "[ERROR] Air not found. Install it with: go install github.com/air-verse/air@v1.63.0"; exit 1; }
	@command -v bun >/dev/null 2>&1 || { echo "[ERROR] Bun not found. Air requires Bun for frontend builds. Install: curl -fsSL https://bun.sh/install | bash"; exit 1; }
	air

# Start full dev environment: dev DB + Redis + Air
dev-full: dev-db-up
	@echo "[INFO] Dev database is ready"
	@echo "[INFO] Starting Air with full dev environment..."
	@$(MAKE) dev

## Dev DB (default profile)
dev-db-up:
	$(COMPOSE) -f $(COMPOSE_FILE) --profile dev up -d postgres

dev-db-down:
	$(COMPOSE) -f $(COMPOSE_FILE) --profile dev down

# Danger: also removes volumes
dev-db-destroy:
	$(COMPOSE) -f $(COMPOSE_FILE) --profile dev down -v

# Seed development database with sample data
dev-db-seed:
	@echo "Seeding development database..."
	go run ./cmd/merrymaker-admin db-seed

# Verify development database has seeded data
dev-db-verify:
	@./scripts/verify-dev-data.sh

# Clear seen-domain cache (DB + Redis)
CLEAR_SEEN_FLAGS :=
ifdef SITE_ID
CLEAR_SEEN_FLAGS += --site-id $(SITE_ID)
endif
ifdef SCOPE
CLEAR_SEEN_FLAGS += --scope $(SCOPE)
endif
ifdef DOMAIN
CLEAR_SEEN_FLAGS += --domain $(DOMAIN)
endif
ifdef ALL
CLEAR_SEEN_FLAGS += --all
endif
ifdef DRY_RUN
CLEAR_SEEN_FLAGS += --dry-run
endif
ifdef YES
CLEAR_SEEN_FLAGS += --yes
endif

.PHONY: clear-seen-domains
clear-seen-domains:
	@echo "[INFO] Clearing seen domain cache with flags: $(CLEAR_SEEN_FLAGS)"
	go run ./cmd/merrymaker-admin clear-seen-domains $(CLEAR_SEEN_FLAGS)

## Test DB (isolated profile)
test-db-up:
	$(COMPOSE) -f $(COMPOSE_FILE) --profile test up -d postgres_test
	$(MAKE) wait-test-db

# Stop only test services and remove volumes for clean state
test-db-down:
	$(COMPOSE) -f $(COMPOSE_FILE) --profile test down -v

# Bring up full test infra (Postgres + Redis)
.PHONY: test-infra-up
test-infra-up:
	$(COMPOSE) -f $(COMPOSE_FILE) --profile test up -d postgres_test redis_test
	$(MAKE) wait-test-db

# Run tests with full test infra up (allows Redis-dependent tests)
.PHONY: test-with-infra
test-with-infra: test-infra-up
	TEST_DB_HOST=$(TEST_DB_HOST) TEST_DB_PORT=$(TEST_DB_PORT) \
	TEST_DB_USER=$(TEST_DB_USER) TEST_DB_PASSWORD=$(TEST_DB_PASSWORD) \
	TEST_DB_NAME=$(TEST_DB_NAME) DB_SSL_MODE=disable TEST_DB_EPHEMERAL=1 \
	go test $(PKG) $(TESTARGS)

# Strict full integration: require infra (fail if missing); good for CI gates
.PHONY: test-full
test-full: test-infra-up
	TEST_REQUIRE_INFRA=1 \
	TEST_DB_HOST=$(TEST_DB_HOST) TEST_DB_PORT=$(TEST_DB_PORT) \
	TEST_DB_USER=$(TEST_DB_USER) TEST_DB_PASSWORD=$(TEST_DB_PASSWORD) \
	TEST_DB_NAME=$(TEST_DB_NAME) DB_SSL_MODE=disable TEST_DB_EPHEMERAL=1 \
	go test $(PKG) $(TESTARGS)

# Convenience: run workflow tests in http package with infra up
.PHONY: test-workflows
test-workflows: test-infra-up
	TEST_DB_HOST=$(TEST_DB_HOST) TEST_DB_PORT=$(TEST_DB_PORT) \
	TEST_DB_USER=$(TEST_DB_USER) TEST_DB_PASSWORD=$(TEST_DB_PASSWORD) \
	TEST_DB_NAME=$(TEST_DB_NAME) DB_SSL_MODE=disable TEST_DB_EPHEMERAL=1 \
	go test ./internal/http -run 'Workflow_'


# Wait for test DB to be healthy
wait-test-db:
	@echo "Waiting for test DB to be healthy on $(TEST_DB_HOST):$(TEST_DB_PORT)...";
	@timeout=60; \
	if command -v pg_isready >/dev/null 2>&1; then \
	  for i in $$(seq 1 $$timeout); do \
	    if pg_isready -h $(TEST_DB_HOST) -p $(TEST_DB_PORT) -U $(TEST_DB_USER) -d $(TEST_DB_NAME) >/dev/null 2>&1; then \
	      echo "DB is ready"; exit 0; \
	    fi; \
	    sleep 1; \
	  done; \
	  echo "ERROR: Timed out waiting for test DB to be ready on $(TEST_DB_HOST):$(TEST_DB_PORT)"; exit 1; \
	else \
	  echo "pg_isready not found locally; using docker compose exec to check container health"; \
	  for i in $$(seq 1 $$timeout); do \
	    if $(COMPOSE) -f $(COMPOSE_FILE) --profile test exec -T postgres_test pg_isready -U $$POSTGRES_USER -d $$POSTGRES_DB >/dev/null 2>&1; then \
	      echo "DB is ready"; exit 0; \
	    fi; \
	    sleep 1; \
	  done; \
	  echo "ERROR: Timed out waiting for postgres_test container to be ready"; exit 1; \
	fi

# Run tests with TEST_DB_* env set (default to ephemeral per-test schemas)
test: test-db-up
	TEST_DB_HOST=$(TEST_DB_HOST) TEST_DB_PORT=$(TEST_DB_PORT) \
	TEST_DB_USER=$(TEST_DB_USER) TEST_DB_PASSWORD=$(TEST_DB_PASSWORD) \
	TEST_DB_NAME=$(TEST_DB_NAME) DB_SSL_MODE=disable TEST_DB_EPHEMERAL=1 \
	go test $(PKG) $(TESTARGS)

# Run tests using ephemeral per-test schemas (set a hint env, optional for future toggles)
test-ephemeral: test-db-up
	TEST_DB_HOST=$(TEST_DB_HOST) TEST_DB_PORT=$(TEST_DB_PORT) \
	TEST_DB_USER=$(TEST_DB_USER) TEST_DB_PASSWORD=$(TEST_DB_PASSWORD) \
	TEST_DB_NAME=$(TEST_DB_NAME) DB_SSL_MODE=disable TEST_DB_EPHEMERAL=1 \
	go test $(PKG) $(TESTARGS)



.PHONY: lint lint-fix frontend-test frontend-lint frontend-build

# Run linters
lint:
	golangci-lint run

# Run linters and auto-fix where possible
lint-fix:
	golangci-lint run --fix

## Frontend targets

# Run frontend JavaScript tests
frontend-test:
	@echo "[INFO] Running frontend JavaScript tests..."
	@command -v bun >/dev/null 2>&1 || { echo "[ERROR] Bun not found. Install: curl -fsSL https://bun.sh/install | bash"; exit 1; }
	cd frontend && bun run test

# Run frontend linter
frontend-lint:
	@echo "[INFO] Running frontend linter (Biome)..."
	@command -v bun >/dev/null 2>&1 || { echo "[ERROR] Bun not found. Install: curl -fsSL https://bun.sh/install | bash"; exit 1; }
	cd frontend && bun run lint

# Build frontend assets
frontend-build:
	@echo "[INFO] Building frontend assets..."
	@command -v bun >/dev/null 2>&1 || { echo "[ERROR] Bun not found. Install: curl -fsSL https://bun.sh/install | bash"; exit 1; }
	cd frontend && bun install --frozen-lockfile && bun run build
