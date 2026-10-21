# Merrymaker

Merrymaker is a web application security monitoring platform. It runs JavaScript in
instrumented Chromium sessions, captures the events those sessions produce, and
evaluates the captured data against detection rules.

[![CI](https://img.shields.io/badge/CI-GitHub_Actions-blue?logo=githubactions)](https://github.com/target/mmk-ui-api/actions)
[![License](https://img.shields.io/badge/License-Apache_2.0-blue.svg)](LICENSE.txt)

## Overview

The repository contains two services.

| Service | Responsibility |
| --- | --- |
| `merrymaker-go` | HTTP API, rules engine, scheduler, reaper, and web UI. Owns PostgreSQL and Redis state. |
| `puppeteer-worker` | Reserves jobs from the API, drives Chromium, captures events, and ships them back to the API. |

A scheduled or manual trigger creates a job. A worker reserves the job, executes the
source script in a browser session, and captures network, console, and screenshot
events. The rules engine evaluates those events and writes an alert when a rule
matches. Alert sinks deliver the alert as an HTTP webhook.

## Architecture

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

Both services run as separate processes. `merrymaker-go` also runs the rules engine,
scheduler, and reaper as supervised components in the same process, and you can run
any subset of them.

### Data flow

1. A site schedules a job from a cron expression, or you trigger the job from the web UI.
2. A worker reserves the job, renews its lease while the job runs, and executes the source script.
3. The worker captures network, console, and screenshot events, then ships them to `/api/events/bulk`.
4. The rules engine evaluates the captured events and writes an alert when a rule matches.
5. A matching alert is delivered to each enabled HTTP alert sink.

### Domain model

| Entity | Description |
| --- | --- |
| Site | A monitored web application, its scheduling configuration, and its allowlist. |
| Source | A versioned browser automation script. |
| Job | A queued work item with a type, priority, status, and JSON payload. |
| Event | A captured browser event, linked to the job that produced it. |
| Alert | A rule match with context, rule name, and severity. |
| Rule | A detection rule. Types are `unknown_domain`, `ioc_domain`, `yara_rule`, and `custom`. |
| IOC | An indicator of compromise, used by `ioc_domain` rules. |
| Alert sink | An HTTP webhook destination for alerts. |
| Secret | An encrypted credential that a source resolves at job build time. |

YARA is modeled, not implemented. The `yara_rule` type and the `yara_results` columns
exist, and no YARA scanner ships in this repository.

## Requirements

| Component | Version in this repository | Notes |
| --- | --- | --- |
| Go | 1.26.3 (`services/merrymaker-go/go.mod`) | The build image uses `golang:1.27.2-bookworm`. |
| Node.js | 24 in CI, 26.1.0 in the worker image | Required by `puppeteer-worker`. |
| Bun | 1.3.13 | Builds the `merrymaker-go` frontend. |
| PostgreSQL | 15.14 | Started by `make dev-db-up`. |
| Redis | 7 | Started by `make dev-db-up`. |
| Docker | Current release | Runs PostgreSQL and Redis locally. |

## Get started

The steps below run both services on one machine against local PostgreSQL and Redis.

### 1. Start PostgreSQL and Redis

```bash
cd services/merrymaker-go
make dev-db-up
make dev-db-seed
make dev-db-verify
```

`make dev-db-seed` runs migrations, then inserts sample secrets, sources, sites, alert
sinks, and scheduled jobs.

### 2. Configure the service

```bash
cp services/merrymaker-go/.env.example services/merrymaker-go/.env
```

The example file sets `AUTH_MODE=mock`, which issues development sessions without an
identity provider. Set `AUTH_MODE=oauth` for any deployed environment.

### 3. Build the frontend

The Go service embeds its frontend assets, so build them before you start the service.

```bash
cd services/merrymaker-go/frontend
bun install
bun run build
```

### 4. Start the service

```bash
cd services/merrymaker-go
go run ./cmd/merrymaker
```

The web UI listens on `http://localhost:8080`.

`SERVICES` selects the components to run. When `SERVICES` is unset, the service starts
every component.

```bash
SERVICES=http go run ./cmd/merrymaker                          # HTTP API only
SERVICES=http,rules-engine go run ./cmd/merrymaker             # HTTP API and rules engine
SERVICES=scheduler go run ./cmd/merrymaker                     # Scheduler only
SERVICES=reaper go run ./cmd/merrymaker                        # Reaper only
SERVICES=http,scheduler,rules-engine,reaper go run ./cmd/merrymaker
```

In PowerShell, set the variable first: `$env:SERVICES="http"; go run ./cmd/merrymaker`.

### 5. Start a worker

```bash
cd services/puppeteer-worker
npm install
cp .env.example .env
```

Set `MERRYMAKER_API_BASE` in `.env` to the service address, then start the worker.

```bash
npm run worker
```

The worker registers as a `browser` job consumer and logs each reserve, execute, and
report cycle.

## Configuration

Both services read environment variables. `merrymaker-go` loads `.env` from the
working directory at startup, so run it from `services/merrymaker-go`. The container
entry point also expands secrets mounted under `/run/secrets`.
`puppeteer-worker` reads `.env` or a YAML file.

### merrymaker-go

| Variable | Purpose |
| --- | --- |
| `SERVICES` | Comma-separated components: `http`, `rules-engine`, `scheduler`, `reaper`. |
| `HTTP_ADDR` | Listen address for the UI and API. Default is `:8080`. |
| `DB_HOST`, `DB_PORT`, `DB_USER`, `DB_PASSWORD`, `DB_NAME` | PostgreSQL connection. |
| `DB_SSL_MODE` | PostgreSQL TLS mode. Use `require` for deployed databases. |
| `DB_RUN_MIGRATIONS_ON_START` | Apply migrations when the service starts. |
| `REDIS_URI`, `REDIS_PASSWORD` | Redis connection. Sentinel and cluster modes take `REDIS_SENTINEL_*` and `REDIS_CLUSTER_*` variants. |
| `CACHE_REDIS_ADDR`, `CACHE_SOURCE_TTL` | Cache connection and source cache lifetime. |
| `AUTH_MODE` | `oauth` or `mock`. Use `oauth` everywhere except local development. |
| `OAUTH_DISCOVERY_URL`, `OAUTH_CLIENT_ID`, `OAUTH_CLIENT_SECRET`, `OAUTH_REDIRECT_URL`, `OAUTH_SCOPE` | OIDC provider settings. |
| `ADMIN_GROUP`, `USER_GROUP` | Provider groups that map to the admin and user roles. Both are required with `AUTH_MODE=oauth`. |
| `SECRETS_ENCRYPTION_KEY` | Key that encrypts stored secrets. Required before you store any secret. |
| `RULES_ENGINE_CONCURRENCY`, `RULES_ENGINE_BATCH_SIZE`, `RULES_ENGINE_AUTO_ENQUEUE` | Rules engine throughput and automatic enqueue. |
| `SCHEDULER_INTERVAL`, `SCHEDULER_BATCH_SIZE`, `SCHEDULER_MAX_RETRIES` | Scheduler polling and retry limits. |
| `REAPER_INTERVAL`, `REAPER_PENDING_MAX_AGE`, `REAPER_COMPLETED_MAX_AGE`, `REAPER_FAILED_MAX_AGE` | Reaper cadence and retention windows. |

### puppeteer-worker

| Variable | Purpose |
| --- | --- |
| `MERRYMAKER_API_BASE` | Service address the worker calls. |
| `WORKER_JOB_TYPE` | Job type the worker consumes. Default is `browser`. |
| `WORKER_LEASE_SECONDS`, `WORKER_WAIT_SECONDS`, `WORKER_HEARTBEAT_SECONDS` | Lease duration, long-poll wait, and heartbeat interval. |
| `PUPPETEER_HEADLESS`, `PUPPETEER_TIMEOUT` | Browser mode and navigation timeout in milliseconds. |
| `FILE_CAPTURE_ENABLED`, `FILE_CAPTURE_TYPES`, `FILE_CAPTURE_MAX_SIZE`, `FILE_CAPTURE_STORAGE` | Response body capture. Storage backends are `memory` and `redis`. |
| `SHIPPING_ENDPOINT`, `SHIPPING_BATCH_SIZE`, `SHIPPING_MAX_BATCH_AGE` | Event shipping target and batching. |
| `CLIENT_MONITORING_ENABLED`, `CLIENT_MONITORING_EVENTS` | In-page monitoring of browser APIs. |

For the complete list, including YAML keys, see
[`services/puppeteer-worker/README.md`](services/puppeteer-worker/README.md).

## Development

### Tests

| Command | Scope |
| --- | --- |
| `make test` | Go tests. Starts a test PostgreSQL instance, runs the suite, then tears the instance down. |
| `make test-full` | Go tests with `TEST_REQUIRE_INFRA=1`, so tests that need PostgreSQL or Redis run instead of skipping. |
| `make lint`, `make lint-fix` | `golangci-lint`. CI additionally runs `gosec`, `govulncheck`, and CodeQL. |
| `npm test` in `services/puppeteer-worker` | TypeScript build, then `node --test`. |
| `bun run test` in `services/merrymaker-go/frontend` | Frontend tests. |

### Live reload

```bash
cd services/merrymaker-go
go install github.com/air-verse/air@v1.63.0
make dev-full     # dependencies plus live reload
make dev          # live reload only
```

Air rebuilds on Go, template, style, and vendor changes. Set `DEV=true` so the service
reads templates and static assets from disk instead of the embedded copies.

### Database targets

| Command | Effect |
| --- | --- |
| `make dev-db-up`, `make dev-db-down` | Start or stop the development database. |
| `make dev-db-seed`, `make dev-db-verify` | Apply migrations with sample data, then verify the data. |
| `make dev-db-destroy` | Delete the development database volume. |
| `make test-db-up`, `make test-db-down` | Start or stop the test database. |

### Frontend

```bash
cd services/merrymaker-go/frontend
bun install
bun run watch     # development build with watch
bun run build     # production build
bun run lint
bun run format
```

### Administrative CLI

`cmd/merrymaker-admin` operates on service data without going through the UI.

| Command | Effect |
| --- | --- |
| `migrate`, `db-seed`, `db-reset` | Apply migrations, seed development data, or reset the schema. |
| `clear-seen-domains`, `list-seen-domains` | Clear or inspect the seen-domain cache in PostgreSQL and Redis. |
| `list-alertonce-keys`, `clear-alertonce-keys` | Inspect or clear alert deduplication keys in Redis. |
| `fire-http-alert` | Create and dispatch an alert to the configured sinks. |
| `rules-job-results` | Inspect cached rules results for a job. |

Run `go run ./cmd/merrymaker-admin` with no arguments to print usage.

## API

### Endpoint audiences

| Namespace | Caller | Protection |
| --- | --- | --- |
| `/`, `/dashboard`, `/alerts`, `/sites`, `/sources`, `/jobs`, `/secrets`, `/alert-sinks`, `/iocs`, `/allowlist` | Browser | Session cookie. Administrative mutations also require the admin role and a CSRF token. |
| `/auth/*` | Browser | Login, callback, and logout. |
| `/static/*`, `/favicon.ico`, `/healthz` | Browser, load balancer | Static assets and health checks. |
| `/api/*` | `puppeteer-worker` | Cluster-internal transport. No session or CSRF protection. See [Secure deployment](#secure-deployment). |

### Service API

| Method and path | Purpose |
| --- | --- |
| `POST /api/jobs` | Create a job. |
| `GET /api/jobs/{type}/reserve_next` | Reserve the next job of a type. Accepts `lease` and `wait`. |
| `GET /api/jobs/{type}/stats` | Queue statistics for a job type. |
| `GET /api/jobs/{id}/status` | Job status. |
| `GET /api/jobs/{id}/events` | Events captured for a job. |
| `GET /api/jobs/{id}/rules-results` | Cached rules results for a job. |
| `POST /api/jobs/{id}/heartbeat` | Extend the job lease. Accepts `extend`. |
| `POST /api/jobs/{id}/complete` | Mark the job complete. |
| `POST /api/jobs/{id}/fail` | Mark the job failed. |
| `POST /api/events/bulk` | Insert captured events. |
| `POST /api/iocs/bulk` | Import indicators of compromise. |
| `GET /api/iocs/stats` | Indicator counts. |
| `POST /api/sources` | Create a source. |

### Job payloads

A payload takes a `script` string or a `url`. Use `url` for scheduled scans and
`script` for authored sources.

```json
{
  "script": "await page.goto('https://example.com'); await screenshot();",
  "source_id": "6f1a1f1c-0000-4000-8000-000000000000"
}
```

```json
{
  "url": "https://example.com"
}
```

## Secure deployment

Merrymaker ships services with different trust levels. The deployment decides which
of them a browser can reach, which are reachable only from other services, and which
networks the worker can reach.

### Responsibilities

| Control | Implemented in | Owner |
| --- | --- | --- |
| Session authentication for the web UI | `merrymaker-go` | Merrymaker |
| Admin role and CSRF checks for administrative UI mutations | `merrymaker-go` | Merrymaker |
| Admin role checks for the secrets, alert sink, and IOC resources | `merrymaker-go` | Merrymaker |
| Restricting the service API to cluster traffic | Gateway configuration | Deployment |
| Private reachability of PostgreSQL and Redis | Network policy | Deployment |
| Outbound egress for instrumented browsers | Worker configuration | Deployment |
| OAuth provider, group mapping, and token validation | `AUTH_MODE`, `OAUTH_*`, `ADMIN_GROUP`, `USER_GROUP` | Deployment |

### Network exposure

The web UI and the service API share one listener. The service API is the transport
that the worker uses to reserve jobs, renew leases, report results, and ship events.
It is not a browser interface, and it does not apply the session and CSRF protections
that the UI namespace applies.

**Note:** Keeping the service API off browser-reachable networks is a deployment
responsibility. Publishing `/api/*` through the same gateway rule as the UI is not a
supported configuration.

Recommended gateway policy:

- Route `/`, `/dashboard`, `/alerts`, `/sites`, `/sources`, `/jobs`, `/secrets`,
  `/alert-sinks`, `/iocs`, and `/auth/*` to the UI.
- Route `/api/*` only to service backends on a private network.
- Serve `/static/*` and `/healthz` alongside the UI.
- Do not publish `/api/*` on a public address.

### Authentication and roles

`AUTH_MODE` accepts `oauth` or `mock`. Use `oauth` in every deployed environment.

**Warning:** `AUTH_MODE=mock` bypasses the identity provider and issues development
sessions. Use it for local development and automated tests only.

- UI reads require a browser session.
- Administrative UI mutations, including source and secret creation, require the admin role and a CSRF token.
- The secrets, alert sink, and IOC resources under `/api/*` require the admin role.
- Roles map from the provider groups named by `ADMIN_GROUP` and `USER_GROUP`. Both are required with `AUTH_MODE=oauth`.

### Browser egress

The worker visits third-party pages in instrumented Chromium instances. Constrain the
networks those pages reach, independently of their own policy.

- Route browser traffic through your egress proxy with `launch.args`, for example
  `--proxy-server=socks5://<proxy-host>:1180`. See
  [`services/puppeteer-worker/README.md`](services/puppeteer-worker/README.md).
- Puppeteer 25 accepts `launch.blocklist` and `launch.allowlist`. Both take
  URLPattern strings and are enforced in the browser's network service. A blocklist
  that covers loopback addresses, private address ranges, and cluster service names
  adds a second layer.
- Proxy configuration applies to the browser process. It does not constrain the
  worker process, which reaches the service API directly to reserve jobs and ship
  events.

**Note:** Puppeteer documents network allowlists and blocklists as guardrails, not as
a network sandbox. Use container or operating system controls when you need an
isolation guarantee.

### Sources and job payloads

A source is an automation script that the worker executes. Treat source authorship as
equivalent to read access over the secrets that the source names.

- Create and edit sources from the web UI, which requires the admin role.
- A source references stored secrets by name using `__NAME__` placeholders. The
  service resolves the placeholders when it builds the job payload.
- Job payloads accept a `script` string or a `url`. Prefer `url` payloads for
  scheduled scans and reserve `script` payloads for authored sources.

### Data stores

PostgreSQL and Redis hold jobs, events, captured artifacts, and encrypted secrets.
Keep both on private networks and grant the credentials only to the services that
need them.

### Pre-deployment checklist

1. Set `AUTH_MODE=oauth`, then configure `OAUTH_DISCOVERY_URL`, `OAUTH_CLIENT_ID`,
   `OAUTH_CLIENT_SECRET`, `OAUTH_REDIRECT_URL`, `ADMIN_GROUP`, and `USER_GROUP`.
2. Set `SECRETS_ENCRYPTION_KEY` before you store any secret.
3. Confirm that a browser outside the cluster cannot reach `/api/*`.
4. Confirm that PostgreSQL and Redis are unreachable from outside the cluster.
5. Configure browser egress and verify it with a test source.
6. Confirm that source authorship is limited to the admin role.

## Project layout

```
merrymaker/
├── services/
│   ├── merrymaker-go/              # Core Go service
│   │   ├── cmd/
│   │   │   ├── merrymaker/         # Service entry point
│   │   │   └── merrymaker-admin/   # Administrative CLI
│   │   ├── config/                 # Configuration loading
│   │   ├── internal/
│   │   │   ├── domain/             # Entities, value objects, and rule models
│   │   │   ├── ports/              # Interfaces for persistence and external systems
│   │   │   ├── service/            # Scheduler, rules engine, reaper orchestration
│   │   │   ├── core/               # Shared infrastructure primitives
│   │   │   ├── data/               # PostgreSQL and Redis repositories
│   │   │   ├── adapters/           # Auth, job runners, notifications
│   │   │   ├── http/               # Handlers, middleware, routes, templates
│   │   │   ├── bootstrap/          # Composition root
│   │   │   ├── migrate/            # Migration helpers and SQL
│   │   │   ├── mocks/              # Generated gomock stubs
│   │   │   └── testutil/           # Test harnesses
│   │   ├── frontend/               # Bun-based HTMX frontend
│   │   ├── assets.go               # Embed bindings for frontend assets
│   │   └── Makefile                # Development orchestration
│   └── puppeteer-worker/
│       ├── src/                    # TypeScript source
│       ├── config/                 # Configuration schema
│       └── scripts/                # Build scripts
└── .github/workflows/              # CI, security scanning, image builds
```

`assets.go` embeds the built frontend, which replaces the `web/` directory used by
earlier releases.

## Documentation

- [`services/puppeteer-worker/README.md`](services/puppeteer-worker/README.md): worker configuration, storage backends, and event shipping.
- [`services/merrymaker-go/frontend/README.md`](services/merrymaker-go/frontend/README.md): frontend build system.

## License

Apache License 2.0. See [`LICENSE.txt`](LICENSE.txt).
