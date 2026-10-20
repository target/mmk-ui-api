# Merrymaker

A distributed browser automation and security monitoring platform for detecting threats in web applications through instrumented browser sessions.

[![Go 1.24.6+](https://img.shields.io/badge/Go-1.24.6+-00ADD8?style=flat&logo=go)](https://golang.org/)
[![Node 20.18.1+](https://img.shields.io/badge/Node-20.18.1+-339933?style=flat&logo=node.js)](https://nodejs.org/)
[![PostgreSQL 15+](https://img.shields.io/badge/PostgreSQL-15+-4169E1?style=flat&logo=postgresql&logoColor=white)](https://www.postgresql.org/)
[![Redis 7+](https://img.shields.io/badge/Redis-7+-DC382D?style=flat&logo=redis&logoColor=white)](https://redis.io/)
[![htmx](https://img.shields.io/badge/htmx-2.0-3D72D7?style=flat&logo=htmx&logoColor=white)](https://htmx.org/)
[![Architecture: Hexagonal](https://img.shields.io/badge/Architecture-Hexagonal-blue)](https://alistair.cockburn.us/hexagonal-architecture/)
[![CI](https://img.shields.io/badge/CI-GitHub_Actions-blue?logo=githubactions)](https://github.com/target/mmk-ui-api/actions)

## Overview

Merrymaker is a security monitoring platform that executes JavaScript in instrumented browser environments to capture and analyze security-relevant events. It consists of two main services:

- **merrymaker-go**: Core API, job queue, rules engine, and web UI
- **puppeteer-worker**: Browser automation worker that executes jobs and captures events

## Features

### Core Platform (merrymaker-go)

- **Job Queue System** - Reliable job processing with priority queues, retries, and scheduling
- **Rules Engine** - Distributed detection system for security threats and anomalies
  - Unknown domain detection with allowlist support
  - IOC (Indicator of Compromise) domain matching
  - YARA rule scanning (planned)
- **Site Management** - Configure and schedule browser automation jobs per site
- **Source Management** - Store and version browser automation scripts
- **Alert System** - HTTP webhooks for security alert notifications
- **Event Storage** - Capture and query browser events (network, console, screenshots)
- **Secrets Management** - Encrypted storage for credentials and API keys
- **Web UI** - Modern interface for managing sites, sources, jobs, and alerts
- **Multi-Service Architecture** - Run HTTP API, rules engine, scheduler, and reaper independently or together

### Browser Worker (puppeteer-worker)

- **Browser Automation** - Execute JavaScript in instrumented Chromium instances
- **Event Capture** - Network requests/responses, console logs, screenshots, client-side monitoring
- **File Capture** - Capture and store response bodies with deduplication
- **Flexible Storage** - Memory, Redis, or cloud storage backends for captured files
- **Job Processing** - Reserve, execute, and report job status via REST API
- **Configurable** - YAML and environment variable configuration

## Quick Start

### Prerequisites

- Go 1.24.6+
- Node.js 20.18.1+
- Bun 1.2+
- PostgreSQL 15+
- Redis 7+
- Docker & Docker Compose (for local development)

Use Bun for the `merrymaker-go` frontend build pipeline; Node.js remains required for the `puppeteer-worker` service and other Node-based tooling in this repository.

### 1. Start Infrastructure

```bash
# Start PostgreSQL and Redis for development
cd services/merrymaker-go
make dev-db-up

# Wait for services to be healthy, then run migrations and seed data
make dev-db-seed
make dev-db-verify
```

### 2. Configure Environment

```bash
# Copy example environment file
cp services/merrymaker-go/.env.example services/merrymaker-go/.env

# Edit .env with your configuration
# Required: DB credentials, Redis connection, OAuth settings
# Local dev: set AUTH_MODE=mock to use the built-in mock identity provider (never enable in production)
```

### 3. Start Merrymaker Service

```bash
cd services/merrymaker-go

# Build frontend assets (requires Bun)
cd frontend
bun install
bun run build
cd ..

# Run all services (HTTP API, rules engine, scheduler, reaper)
go run ./cmd/merrymaker

# Or run specific services
SERVICES=http go run ./cmd/merrymaker                    # HTTP API only
SERVICES=http,rules-engine go run ./cmd/merrymaker       # HTTP + rules engine
SERVICES=scheduler go run ./cmd/merrymaker               # Scheduler only
SERVICES=reaper go run ./cmd/merrymaker                  # Cleanup worker only
SERVICES=http,scheduler,rules-engine,reaper go run ./cmd/merrymaker  # Explicit full set
```

Default behavior (no `SERVICES` set) starts all available components. On Windows PowerShell use `$env:SERVICES="http"; go run ./cmd/merrymaker`; on `cmd.exe` use `set SERVICES=http && go run .\cmd\merrymaker`.

The web UI will be available at `http://localhost:8080`

### 4. Start Puppeteer Worker

```bash
cd services/puppeteer-worker

# Install dependencies
npm install

# Configure worker
cp .env.example .env
# Edit .env: set MERRYMAKER_API_BASE=http://localhost:8080

# Start worker
npm run worker
```

## Architecture

### Service Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Merrymaker Platform                     │
├─────────────────────────────────────────────────────────────┤
│                                                             │
│  ┌──────────────┐  ┌──────────────┐  ┌──────────────┐       │
│  │   HTTP API   │  │ Rules Engine │  │  Scheduler   │       │
│  │              │  │              │  │              │       │
│  │ • Web UI     │  │ • Unknown    │  │ • Cron jobs  │       │
│  │ • REST API   │  │   domains    │  │ • Site scans │       │
│  │ • Auth       │  │ • IOC match  │  │ • Recurring  │       │
│  └──────┬───────┘  └──────┬───────┘  └──────┬───────┘       │
│         │                 │                │                │
│         └─────────────────┼────────────────┘                │
│                           │                                 │
│         ┌─────────────────┴──────────────────┐              │
│         │                                    │              │
│    ┌────▼─────┐                         ┌────▼─────┐        │
│    │PostgreSQL│                         │  Redis   │        │
│    │          │                         │          │        │
│    │• Jobs    │                         │• Caches  │        │
│    │• Events  │                         │• Locks   │        │
│    │• Sites   │                         │• Files   │        │
│    └──────────┘                         └──────────┘        │
│                                                             │
└───────────────────────────┬─────────────────────────────────┘
                            │
                            │ REST API
                            │
                ┌───────────▼───────────┐
                │  Puppeteer Worker(s)  │
                │                       │
                │ • Reserve jobs        │
                │ • Execute scripts     │
                │ • Capture events      │
                │ • Report results      │
                └───────────────────────┘
```

### Data Flow

1. **Job Creation**: Sites schedule jobs via cron expressions or manual triggers
2. **Job Execution**: Puppeteer workers reserve jobs, execute browser scripts
3. **Event Capture**: Workers capture network, console, screenshot events
4. **Event Processing**: Rules engine analyzes events for security threats
5. **Alert Generation**: Detected threats trigger alerts sent to HTTP webhooks

### Domain Models

**Core Entities:**

- **Sites**: Monitored web applications with scheduling configuration
- **Sources**: Versioned browser automation scripts
- **Jobs**: Queued work items with priority, status, and payload
- **Events**: Browser events (network, console, screenshots) linked to jobs
- **Alerts**: Fired security alerts with context and metadata
- **Rules Engine**: Detection rules (unknown domains, IOC matching, YARA)
- **Secrets**: Encrypted credentials for authentication
- **HTTP Alert Sinks**: Webhook destinations for alert notifications

## Configuration

### Merrymaker Service

Configuration via environment variables (`.env` file):

```bash
# Database
DB_HOST=localhost
DB_PORT=5432
DB_NAME=merrymaker
DB_USER=merrymaker
DB_PASSWORD=merrymaker

# Redis
REDIS_HOST=localhost
REDIS_PORT=6379

# Authentication
AUTH_MODE=oauth                    # oauth or mock (mock is for local development only)
OAUTH_ISSUER=https://auth.example.com
OAUTH_CLIENT_ID=your-client-id
OAUTH_CLIENT_SECRET=your-secret
ADMIN_GROUP=merrymaker-admins
USER_GROUP=merrymaker-users

# Service Configuration
SERVICES=http,rules-engine,scheduler,reaper  # Comma-separated list
HTTP_ADDR=:8080

# Rules Engine
RULES_ENGINE_CONCURRENCY=1
RULES_ENGINE_BATCH_SIZE=100
RULES_ENGINE_AUTO_ENQUEUE=true

# Scheduler
SCHEDULER_CONCURRENCY=1
SCHEDULER_BATCH_SIZE=10
SCHEDULER_INTERVAL=30s

# Reaper
REAPER_INTERVAL=5m
REAPER_PENDING_MAX_AGE=1h
REAPER_COMPLETED_MAX_AGE=168h
REAPER_FAILED_MAX_AGE=168h
REAPER_BATCH_SIZE=1000
# Additional guidance: services/merrymaker-go/README.md#reaper-configuration
```

### Puppeteer Worker

Configuration via environment variables or YAML:

```bash
# Worker Configuration
MERRYMAKER_API_BASE=http://localhost:8080
WORKER_JOB_TYPE=browser
WORKER_LEASE_SECONDS=30
WORKER_WAIT_SECONDS=25
WORKER_HEARTBEAT_SECONDS=10

# Browser Configuration
PUPPETEER_HEADLESS=true
PUPPETEER_TIMEOUT=30000

# File Capture
FILE_CAPTURE_ENABLED=true
FILE_CAPTURE_TYPES=script,document
FILE_CAPTURE_STORAGE=redis
FILE_CAPTURE_MAX_SIZE=10485760

# Event Shipping
SHIPPING_BATCH_SIZE=50
SHIPPING_MAX_BATCH_AGE=5000
```

See [`services/puppeteer-worker/README.md`](services/puppeteer-worker/README.md) for detailed configuration options.

## Development

### Project Structure

```
merrymaker/
├── services/
│   ├── merrymaker-go/              # Core Go service
│   │   ├── cmd/                    # Entry points
│   │   ├── internal/               # Domain, application, infrastructure packages
│   │   │   ├── domain/             # Business entities and value objects
│   │   │   ├── ports/              # Interfaces for persistence/external systems
│   │   │   ├── service/            # Scheduler, rules engine, reaper orchestration
│   │   │   ├── core/               # Shared infrastructure primitives
│   │   │   ├── data/               # Repository implementations (Postgres, Redis, etc.)
│   │   │   ├── adapters/           # Integrations (auth, job runners, notifications)
│   │   │   ├── http/               # HTTP handlers, middleware, routes
│   │   │   ├── bootstrap/          # Composition root and wiring
│   │   │   ├── migrate/            # Migration helpers and SQL files
│   │   │   ├── mocks/              # Generated gomock stubs
│   │   │   └── testutil/           # Test harnesses and utilities
│   │   ├── frontend/               # Bun-based HTMX frontend build
│   │   ├── assets.go               # Embed bindings for frontend assets
│   │   └── Makefile                # Dev orchestration
│   │
│   └── puppeteer-worker/           # Browser automation worker
│       ├── src/                    # TypeScript source
│       ├── config/                 # Configuration schemas
│       └── scripts/                # Build scripts
│
└── build/                          # Build artifacts
```

Static assets for `merrymaker-go` are embedded via `assets.go`, replacing the legacy `web/` directory used in earlier releases.

### Running Tests

```bash
# Merrymaker Go tests
cd services/merrymaker-go
make test                       # Run all tests
make test-integration           # Integration tests only

# Puppeteer worker tests
cd services/puppeteer-worker
npm test
```

### Database Management

```bash
cd services/merrymaker-go

# Development database
make dev-db-up                  # Start dev PostgreSQL
make dev-db-down                # Stop dev PostgreSQL
make dev-db-seed                # Seed with sample data
make dev-db-verify              # Verify seeded data

# Test database
make test-db-up                 # Start test PostgreSQL
make test-db-down               # Stop test PostgreSQL
make test                       # Run tests (auto-starts test DB)
```

### Live-Reload Development (Recommended)

For rapid iteration with automatic rebuilds and restarts:

```bash
cd services/merrymaker-go

# Install Air (one-time setup, pinned version)
go install github.com/air-verse/air@v1.63.0

# Start full dev environment (DB + live-reload)
make dev-full

# Or start live-reload only (no DB)
make dev
```

Air watches Go source files, frontend styles, templates, and vendor scripts. Edit any file and see changes in ~1-2 seconds without manual rebuilds or restarts. The `DEV=true` environment variable enables hot-reloading of templates and static assets from disk.

### Frontend Development

```bash
cd services/merrymaker-go/frontend

# Install dependencies
bun install

# Development build (watch mode)
bun run watch

# Production build
bun run build

# Lint and format
bun run lint
bun run format
```

## API Reference

### Endpoint audiences

| Namespace | Audience | Notes |
| --- | --- | --- |
| `/`, `/dashboard`, `/alerts`, `/sites`, `/sources`, `/jobs`, `/secrets`, `/alert-sinks` | Browser | Session authenticated. Administrative mutations also require the admin role and a CSRF token. |
| `/auth/*` | Browser | Login, callback, and logout. |
| `/static/*`, `/favicon.ico`, `/healthz` | Browser or load balancer | Assets and health checks. |
| `/api/*` | Services | Cluster-internal transport between the scanner and the core service. See [Secure deployment](#secure-deployment). |

### REST API Endpoints

**Jobs:**

- `GET /api/jobs/{jobType}/reserve_next?lease={seconds}&wait={seconds}` - Reserve next job
- `POST /api/jobs/{jobId}/heartbeat?extend={seconds}` - Extend job lease
- `POST /api/jobs/{jobId}/complete` - Mark job complete
- `POST /api/jobs/{jobId}/fail` - Mark job failed
- `GET /api/jobs/{jobId}/status` - Get job status
- `GET /api/jobs/{jobId}/events` - Get job events

**Events:**

- `POST /api/events/bulk` - Bulk insert events

**Sites, Sources, Secrets, Alerts:**

- See web UI or OpenAPI documentation (planned)

### Job Payload Format

```json
{
  "script": "await page.goto('https://example.com'); await screenshot();",
  "source_id": "optional-source-uuid"
}
```

Or URL-only format:

```json
{
  "url": "https://example.com"
}
```

## Secure deployment

Merrymaker ships services. The deployment determines which services a browser can
reach, which are reachable only from other services in the cluster, and which
networks the scanner can initiate connections to. Set those boundaries before you
expose an instance.

### Deployment responsibilities

| Control | Enforced in | Owner |
| --- | --- | --- |
| Session authentication for the web UI | `merrymaker-go` | Merrymaker |
| Admin role and CSRF checks for administrative UI mutations | `merrymaker-go` | Merrymaker |
| Admin role checks for the secrets, alert sink, and IOC API resources | `merrymaker-go` | Merrymaker |
| Restricting the service API to cluster traffic | Gateway (Traefik, Portainer) | Deployment |
| Private reachability of PostgreSQL and Redis | Network policy | Deployment |
| Outbound egress for instrumented browsers | Worker configuration | Deployment |
| OAuth issuer, group mapping, and token validation | `AUTH_MODE`, `OAUTH_*`, `ADMIN_GROUP`, `USER_GROUP` | Deployment |

### Network exposure

The web UI and the service API are registered on a single listener. The service
API is the transport that the scanner uses to reserve jobs, renew leases, report
results, and ship captured events. It is not a browser interface, and it does not
carry the session and CSRF protections that the UI namespace applies.

> **Note:** Keeping the service API off browser-reachable networks is a deployment
> responsibility. Publishing `/api/*` through the same gateway rule as the UI is
> not a supported configuration.

Recommended gateway policy:

- Route `/`, `/dashboard`, `/alerts`, `/sites`, `/sources`, `/jobs`, `/secrets`,
  `/alert-sinks`, and `/auth/*` to the UI.
- Route `/api/*` only to service backends on the cluster's private network.
- Serve `/static/*` and `/healthz` alongside the UI, and do not publish `/api/*`
  on a public address.

### Authentication and roles

`AUTH_MODE` accepts `oauth` or `mock`. Use `oauth` in every deployed environment.

> **Warning:** `AUTH_MODE=mock` bypasses the identity provider and issues
> development sessions. Use it for local development and automated tests only.

- UI reads require a browser session.
- Administrative UI mutations, including source and secret creation, require the
  admin role and a CSRF token.
- The secrets, alert sink, and IOC resources under `/api/*` require the admin role.
- Roles map from the groups named by `ADMIN_GROUP` and `USER_GROUP`. Both are
  required when `AUTH_MODE=oauth`.

### Browser egress

The scanner visits third-party pages in instrumented Chromium instances. Constrain
what those pages can reach independently of the policy of the pages themselves.

- Route browser traffic through your egress proxy in `launch.args`, for example
  `--proxy-server=socks5://<proxy-host>:1180`. See
  [`services/puppeteer-worker/README.md`](services/puppeteer-worker/README.md).
- Puppeteer 24.42 and later accept `launch.blocklist`, and 24.43 and later accept
  `launch.allowlist` (Chrome 149 or later). Both take URLPattern strings and are
  enforced in the browser's network service. A blocklist that covers loopback,
  private address ranges, and cluster service names is a useful second layer.
- Proxy configuration applies to the browser process. It does not constrain the
  worker process, which reaches the service API directly to reserve jobs and ship
  events.

> **Note:** Puppeteer documents network allowlists and blocklists as guardrails
> rather than as a network sandbox. Use container or operating system level
> controls when you need an isolation guarantee.

### Sources and job payloads

Sources are automation scripts that the scanner executes on your behalf. Treat
them as trusted operator input.

- Create and edit sources from the web UI, which requires the admin role.
- A source can reference stored secrets by name using `__NAME__` placeholders,
  which the service resolves when it builds the job payload. Source authorship is
  therefore equivalent to read access over the secrets that the source names.
- Job payloads accept either a `script` string or a `url`. Prefer `url` payloads
  for scheduled scans, and reserve `script` payloads for authored sources.

### Data stores

PostgreSQL and Redis hold jobs, events, captured artifacts, and encrypted secrets.
Keep both on private networks and give the credentials only to the services that
need them.

### Pre-deployment checklist

1. Set `AUTH_MODE=oauth` and configure `OAUTH_ISSUER`, `OAUTH_CLIENT_ID`,
   `OAUTH_CLIENT_SECRET`, `ADMIN_GROUP`, and `USER_GROUP`.
2. Confirm that a browser outside the cluster cannot reach `/api/*`.
3. Confirm that PostgreSQL and Redis are unreachable from outside the cluster.
4. Configure browser egress for the scanner and verify it with a test source.
5. Confirm that source authorship is limited to the admin role.

## Documentation

- **[Puppeteer Worker](services/puppeteer-worker/README.md)** - Worker configuration and usage
- **[Frontend Build](services/merrymaker-go/frontend/README.md)** - Frontend build system
