# Puppeteer Worker

A simplified browser automation worker service that executes JavaScript in instrumented browser environments and captures security-relevant events.

### Usage

```typescript
import { WorkerLoop } from './worker-loop.js';
import { loadConfig } from './config-loader.js';

const config = await loadConfig();
const workerLoop = new WorkerLoop({
  worker: config.worker,
  config: config,
});

await workerLoop.start();
```

### Configuration

The worker loop requires these configuration parameters:

- `worker.apiBaseUrl`: Base URL for the job queue API
- `worker.jobType`: Type of jobs to process (`browser` or `rules`)
- `worker.leaseSeconds`: How long to lease jobs (default: 30)
- `worker.waitSeconds`: Long-poll timeout for job reservation (default: 25)
- `worker.heartbeatSeconds`: Interval for sending heartbeats (default: 10)

### Running the Worker

```sh
# Development
npm run worker

# Production (after build)
npm run worker:dist

# With environment file
node --env-file=.env dist/worker-main.js
```

### Container runtime notes
- The Docker image uses `tini` as PID 1 so Chromium child processes are reaped and signals are forwarded cleanly. If you extend the image, keep `tini` (or another minimal init) in place to avoid zombie Chromium helpers during long-running sessions.
- `tini` runs with `-g` to send shutdown signals to the full process group, ensuring Chromium helpers exit promptly during deployments.
- Ensure `/tmp/.chromium` (configured via `XDG_CONFIG_HOME` / `XDG_CACHE_HOME`) remains writable for the container user so repeated browser launches do not fail when Chrome rotates its profile files.
- The image sets `XDG_RUNTIME_DIR=/tmp/.runtime` and creates the directory with 0700 permissions to quiet Chromium runtime warnings and allow socket/lock files; keep this writable if you customize the container.

## Configuration

The worker supports configuration from two sources: environment variables and a YAML file. Configuration is applied in this order: defaults, YAML (if present), then environment variables. Unknown options are rejected.

## Environment variables
Use Node v24 built-in env-file support to populate `process.env`.

```sh
node --env-file=.env dist/index.js
```

Common variables:
- `PUPPETEER_HEADLESS` = `true|false`
- `PUPPETEER_TIMEOUT` = number (ms)
- `FILE_CAPTURE_ENABLED` = `true|false`
- `FILE_CAPTURE_TYPES` = comma-separated values
- `FILE_CAPTURE_CT_MATCHERS` = comma-separated content-types to allow for capture (e.g., application/javascript,text/javascript,application/json)

- `FILE_CAPTURE_MAX_SIZE` = number (bytes)
- `FILE_CAPTURE_STORAGE` = `memory|redis|cloud`
- `SHIPPING_ENDPOINT` = URL
- `SHIPPING_BATCH_SIZE` = number
- `SHIPPING_MAX_BATCH_AGE` = number (ms)
- `CLIENT_MONITORING_ENABLED` = `true|false`
- `CLIENT_MONITORING_EVENTS` = comma-separated values

Worker configuration:
- `MERRYMAKER_API_BASE` = Base URL for job queue API
- `WORKER_JOB_TYPE` = `browser|rules` (default: browser)
- `WORKER_LEASE_SECONDS` = number (default: 30)
- `WORKER_WAIT_SECONDS` = number (default: 25)

Note: If `SHIPPING_ENDPOINT` is not provided, it is derived automatically as `new URL('/api/events/bulk', MERRYMAKER_API_BASE).toString()` to anchor at the origin root.

- `WORKER_HEARTBEAT_SECONDS` = number (default: 10)

## YAML configuration
Set `PUPPETEER_WORKER_CONFIG` to the YAML path or rely on defaults:
- `./config/puppeteer-worker.yaml`
- `./puppeteer-worker.config.yaml`

Example:

```yaml
# config/puppeteer-worker.yaml
headless: false
timeout: 60000
worker:
  apiBaseUrl: "https://jobs.merry.example/api"
  jobType: browser
  leaseSeconds: 45
  waitSeconds: 20
  heartbeatSeconds: 12
fileCapture:
  enabled: true
  types: [script, document, stylesheet]
  contentTypeMatchers:
    - application/javascript
    - application/json
    - text/css
  maxFileSize: 2097152 # 2 MiB
  storage: redis
  storageConfig:
    sentinels:
      - { host: redis-01.internal, port: 26379 }
      - { host: redis-02.internal, port: 26379 }
      - { host: redis-03.internal, port: 26379 }
    masterName: filecap
    password: ${REDIS_PASSWORD}
    sentinelPassword: ${REDIS_SENTINEL_PASSWORD}
    db: 2
    prefix: filecap:prod:
    ttlSeconds: 3600
    hashTtlSeconds: 172800
shipping:
  endpoint: "https://events.merry.example/api/events/bulk"
  batchSize: 150
  maxBatchAge: 7000
clientMonitoring:
  enabled: true
  events: [storage, dynamicCode]
launch:
  executablePath: "/usr/bin/chromium"
  args:
    - "--no-sandbox"
    - "--disable-dev-shm-usage"
    - "--proxy-server=socks5://merrysocks:1180"
  defaultViewport:
    width: 1366
    height: 768
```

## Validation and coercion
- Unknown keys are rejected at every level
- Types are validated; timeouts and sizes must be positive numbers
  - URLs: `shipping.endpoint` and `worker.apiBaseUrl` must be valid URLs; invalid values are rejected

- Coercion rules for YAML values:
  - Booleans: `true|false|1|0|yes|no|on|off` (case-insensitive)
  - Numbers: numeric strings are accepted
  - Arrays: either YAML arrays or comma-separated strings
  - `fileCapture.storage`: normalized to `memory|redis|cloud`
- Browser profile/cache directories:
  - The runtime Docker image exports `XDG_CONFIG_HOME`/`XDG_CACHE_HOME` to `/tmp/.chromium` and pre-creates that directory with writable permissions for the app user. If you extend the image or run the worker elsewhere, ensure Chromium has a writable profile directory (for example by setting the same environment variables or mounting an appropriate volume).
- `fileCapture.storageConfig` keys:
  - `memory`: no keys allowed
  - `redis`: `host`, `port`, `username`, `password`, `db`, `keyPrefix`/`prefix`, `ttlSeconds`, `hashTtlSeconds`, `redisClient` (DI), `sentinels` (array of `{host, port}`), `masterName`, `sentinelPassword`
  - `cloud`: `provider`, `bucket`, `region`, `accessKeyId`, `secretAccessKey`


### Redis storageConfig options

- ttlSeconds: TTL for file content keys (default: 3600 seconds)
- hashTtlSeconds: TTL for dedupe index keys `h:<hash>` (default: 86400 seconds)
- keyPrefix/prefix: Namespace prefix for all keys (default: `filecap:`)
- username/password: Auth credentials
- db: Database index
- sentinels/masterName/sentinelPassword: Sentinel configuration for HA
- redisClient: Provide an existing ioredis client instance (DI); when provided, the storage provider will not close the client on cleanup

Examples:

Single instance

```yaml
fileCapture:
  enabled: true
  storage: redis
  storageConfig:
    host: 127.0.0.1
    port: 6379
    prefix: filecap:prod:
    ttlSeconds: 3600
    hashTtlSeconds: 86400
```

Sentinel

```yaml
fileCapture:
  enabled: true
  storage: redis
  storageConfig:
    sentinels:
      - { host: 10.0.0.11, port: 26379 }
      - { host: 10.0.0.12, port: 26379 }
      - { host: 10.0.0.13, port: 26379 }
    masterName: mymaster
    password: ${REDIS_PASSWORD}
    sentinelPassword: ${SENTINEL_PASSWORD}
    db: 0
    prefix: filecap:prod:
    ttlSeconds: 3600
    hashTtlSeconds: 172800
```

Dependency Injection (tests)

```ts
import Redis from 'ioredis';
import { FileCapture } from './dist/file-capture.js';


const redis = new Redis({ host: '127.0.0.1', port: 6380 });
const fc = new FileCapture({
  enabled: true,
  types: ['script', 'document'],
  storage: 'redis',
  storageConfig: { redisClient: redis, prefix: 'filecap:test:' }
}, 'session-123');
```

## Extending the schema
- Edit `configSchema` in `src/config-schema.ts`
- Add fields by choosing a node type and providing defaults and optional `env` binding
- Arrays accept YAML arrays or comma-separated values from env
- For Puppeteer launch options, use the top-level `launch` object (schemaless and not bound to env). Values here are passed to `puppeteer.launch()` with precedence over defaults.
  - Defaults when omitted or invalid:
    - `headless`: defaults to `config.headless !== false`

    - `args`: defaults to ["--disable-web-security", "--no-sandbox"]
  - Coercion: if `launch.args` is provided as a comma-separated string, it will be split on commas into an array.
- If adding a new storage type or changing allowed `storageConfig` keys, update checks in `src/config-loader.ts`
- Unknown keys remain rejected automatically; precedence stays: defaults < YAML < ENV

Example:
```ts
// src/config-schema.ts
export const configSchema = obj({
  ...
  newFeature: bool({ env: 'NEW_FEATURE_ENABLED', default: false }),
});
```

## Programmatic use
- `loadConfig()` returns the merged, validated config
- `PuppeteerRunner.runWithConfig(script)` loads config and runs the script

## CLI examples
```sh
node --env-file=.env dist/index.js

PUPPETEER_WORKER_CONFIG=/app/config/puppeteer-worker.yaml node dist/index.js
```


## Job payloads and writing scripts

The worker executes "browser" jobs whose payload contains the instructions to run.

Accepted payload shapes:
- String: treated as the script to execute

### File capture matching behavior

- Order of checks: `enabled` → `maxFileSize` → type/mime matching
- Type vs. content-type: a file is captured if EITHER condition is true (OR logic)
  - Resource type matches one of `fileCapture.types` (e.g., script, document, stylesheet), or
  - Response `Content-Type` contains any entry in `fileCapture.contentTypeMatchers`
- Matching details: case-insensitive; parameters like `; charset=utf-8` are tolerated (match by substring)

- Object with `script`: `{ "script": "...", "source_id?": "..." }`
- Object with `url`: `{ "url": "https://example.com" }` (generates a minimal `page.goto(url)` script)


Note on ambiguity:
- The runner selects Node-side mode when the script string contains `page.` or `screenshot(` (substring match). This is a heuristic and may false-positive on comments or string literals. There is no explicit `mode` flag in the payload today. To avoid ambiguity, do not include these substrings in browser-context scripts. If you need explicit control in the future, consider structuring scripts to clearly either use Puppeteer `page` APIs or pure browser DOM APIs.

Execution modes:
- Script contains `page.` or `screenshot(`: runs as Node-side async function with access to Puppeteer `page` and helper functions described below
- Script does not contain these substrings: runs in the page context via `page.evaluate(...)` and has access to browser APIs (no helpers)

Custom helpers (available only in the Node-side mode with `page.` in script):
- `await screenshot(opts?)`: captures a PNG screenshot and emits a `Worker.screenshot` event with base64 image data; limited to 25 screenshots per job (calls beyond the limit are ignored). Options match Puppeteer `page.screenshot` options; encoding is fixed to base64.
- `log(message)`: emits a `Worker.log` event; objects are JSON-serialized when possible.


Notes for screenshot helper:
- Only the provided helper emits a Worker.screenshot event. Calling page.screenshot(...) directly will take a screenshot but will not emit an event.
- Helper signature: screenshot(opts?) — do not pass page as the first argument.
- Limit: at most 25 screenshots per job; additional calls are ignored.

Events produced by scripts (console, network, screenshots, client monitoring etc.) are available to downstream systems via the job events API.

### Example scripts

Basic navigation, log, and screenshot:

```js
await page.goto("https://example.com", { waitUntil: "load" });
log("navigated to example.com");
await screenshot({ fullPage: true });
```

Form interaction:

```js
await page.goto("https://example.com/login", { waitUntil: "domcontentloaded" });
await page.type("#username", "user1");
await page.type("#password", "secret", { delay: 20 });
await Promise.all([
  page.waitForNavigation({ waitUntil: "networkidle2" }),
  page.click("button[type=submit]"),
]);
await screenshot();
```

Browser-context script (no `page.`); use a `url` payload or navigate first, then run DOM JS:

```js
// Payload: { "url": "https://example.com" }
// Script runs in the page context
console.log("Hello from inside the page");
localStorage.setItem("key", "value");
```

### Enqueue payload reference (shapes)

String script:

```json
"await page.goto('https://example.com')"
```

Object with script:

```json
{ "script": "await page.goto('https://example.com'); await screenshot();", "source_id": "source-123" }
```

Object with url:

```json
{ "url": "https://example.com" }
```

Notes on precedence:
- If both `script` and `url` are provided, `script` takes precedence (the `url` field is ignored).
- If only `url` is provided, the worker generates a minimal navigation script (`await page.goto(url)`) and runs it.



## Worker ↔ UI API endpoints

Worker-side job endpoints (base: `MERRYMAKER_API_BASE`):
- GET `/api/jobs/{jobType}/reserve_next?lease={seconds}&wait={seconds}` → 200 with job or 204 when none
- POST `/api/jobs/{jobId}/heartbeat?extend={seconds}` → 200/204 on success
- POST `/api/jobs/{jobId}/complete` → 200/204 on success
- POST `/api/jobs/{jobId}/fail` with `{ "error": string }` → 200/204 on success

UI/job viewer endpoints (used by the frontend):
- GET `/api/jobs/{jobId}/events?limit={n}&offset={n}` → array of events

### Status codes and bodies

- reserve_next: 200 with a JSON job, or 204 when no job is available
- heartbeat: 204 (no body) or 200; on 200, body may contain `{ ok: true }`
- complete: 204 (no body) or 200; on 200, body may contain `{ ok: true }`
- fail: 204 (no body) or 200; on 200, body may contain `{ ok: true }`

Clients should treat both 200 and 204 as success for POSTs; a JSON `{ ok: true }` body is optional.

- GET `/api/jobs/{jobId}/status` → `{ status: "waiting|running|completed|failed|cancelled" }`

Notes:
- The worker logs a short payload preview when reserving/processing jobs; full payloads and emitted events are available via the events endpoint.
- Heartbeats extend the lease while a job is running; if heartbeats stop, the job may be reassigned depending on server policy.


## Event payloads and correlation
- Network events include an optional `requestId` on both `Network.requestWillBeSent` and `Network.responseReceived` payloads.
- For captured response bodies, the runner deterministically attaches the `capturedFile` to the exact `Network.responseReceived` event using `requestId` mapping. A safe fallback to the last response event remains.
- The embedded file context contains `storageProvider` and `storageKey` for downstream retrieval.

Example shape (simplified):

- `Network.requestWillBeSent.payload`:
  - `url`, `requestId`, `method`, `headers`, `resourceType`, `initiatingPage`, ...
- `Network.responseReceived.payload`:
  - `url`, `requestId`, `status`, `headers`, `resourceType`, `bodyType`, `capturedFile?`

This enables downstream consumers to correlate requests/responses and retrieve captured files reliably.
