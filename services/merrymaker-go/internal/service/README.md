# Service Layer

This directory contains the **application/orchestration layer** for merrymaker-go, following hexagonal architecture principles.

## Table of Contents

- [Overview](#overview)
- [Responsibilities](#responsibilities)
- [Architecture Principles](#architecture-principles)
- [Service Pattern](#service-pattern)
- [When to Create a New Service](#when-to-create-a-new-service)
- [Dependency Injection](#dependency-injection)
- [Testing Patterns](#testing-patterns)
- [Common Patterns](#common-patterns)
- [Migration Guide](#migration-guide)

---

## Overview

The service layer is the **single source of business logic and orchestration** in the application. It sits between the transport layer (HTTP handlers, adapters) and the data layer (repositories).

```
┌─────────────────────────────────────────┐
│   HTTP Handlers / Adapters              │  ← Transport Layer
│   (internal/http, internal/adapters)    │
└────────────────┬────────────────────────┘
                 │ depends on
                 ▼
┌─────────────────────────────────────────┐
│   Service Layer                          │  ← YOU ARE HERE
│   (internal/service)                     │
└────────────────┬────────────────────────┘
                 │ depends on
                 ▼
┌─────────────────────────────────────────┐
│   Ports (Interfaces)                     │
│   (internal/ports/repositories.go)       │
└────────────────┬────────────────────────┘
                 │ implemented by
                 ▼
┌─────────────────────────────────────────┐
│   Data Layer / Adapters                  │
│   (internal/data, internal/adapters)     │
└─────────────────────────────────────────┘
```

---

## Responsibilities

### ✅ Service Layer DOES:

- **CRUD operations** with business logic
- **Cross-repository orchestration** (coordinating multiple repositories)
- **Transaction management** (when operations span multiple repositories)
- **Caching strategies** (when to cache, when to invalidate)
- **Async operations** (spawning goroutines, pub/sub patterns)
- **Business rule enforcement** (validation, normalization, authorization)
- **Error handling and wrapping** (adding context to errors)
- **Pagination defaults** (normalizing limit/offset parameters)
- **Retry logic** (when appropriate)
- **Metrics and logging** (observability)

### ❌ Service Layer DOES NOT:

- **Import from internal/data** (depends on interfaces only)
- **Import from internal/http** (transport depends on service, not vice versa)
- **Import from internal/adapters** (adapters depend on service, not vice versa)
- **Handle HTTP requests/responses** (that's the HTTP layer's job)
- **Parse query parameters** (that's the HTTP layer's job)
- **Render templates** (that's the HTTP layer's job)
- **Implement repository interfaces** (that's the data layer's job)

---

## Architecture Principles

### 1. Dependency Rule

**Dependencies flow inward:** Service layer depends on ports (interfaces), not concrete implementations.

```go
// ✅ GOOD: Depend on interface
type SecretService struct {
    repo ports.SecretRepository  // Interface from internal/ports
}

// ❌ BAD: Depend on concrete implementation
type SecretService struct {
    repo *data.SecretRepo  // Concrete type from internal/data
}
```

### 2. Single Responsibility

Each service is responsible for **one domain** (e.g., secrets, sources, sites).

### 3. Options Pattern

All services use the **Options pattern** for dependency injection (≤3 fields).

### 4. Interface Segregation

Services depend on **minimal interfaces**, not large interfaces with unused methods.

---

## Service Pattern

See [TEMPLATE.go](./TEMPLATE.go) for a fully documented example.

### Basic Structure

```go
// 1. Options struct (≤3 fields)
type SecretServiceOptions struct {
    Repo   ports.SecretRepository
    Logger *slog.Logger
}

// 2. Service struct (private fields)
type SecretService struct {
    repo   ports.SecretRepository
    logger *slog.Logger
}

// 3. Constructor with validation
func NewSecretService(opts SecretServiceOptions) *SecretService {
    if opts.Repo == nil {
        panic("SecretRepository is required")
    }
    return &SecretService{
        repo:   opts.Repo,
        logger: opts.Logger,
    }
}

// 4. Methods (context first, error wrapping)
func (s *SecretService) Create(
    ctx context.Context,
    req types.CreateSecretRequest,
) (*types.Secret, error) {
    secret, err := s.repo.Create(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("create secret: %w", err)
    }
    return secret, nil
}
```

---

## When to Create a New Service

Create a new service when:

1. **New domain entity** - You're adding a new domain (e.g., webhooks, notifications)
2. **Complex orchestration** - You need to coordinate multiple repositories
3. **Business logic** - You have validation, normalization, or business rules
4. **Caching** - You need to implement caching strategies
5. **Async operations** - You need to spawn goroutines or manage pub/sub

Do NOT create a service for:

- **Pure data access** - If it's just CRUD with no logic, the repository is enough (but we still create services for consistency)
- **HTTP-specific logic** - That belongs in handlers
- **Utility functions** - Create a package in internal/util instead

---

## Dependency Injection

### Rule: ≤3 Parameters

**All services must have ≤3 fields in their Options struct.**

If you need more than 3 dependencies, use a nested config struct:

```go
// ❌ BAD: Too many fields
type EventServiceOptions struct {
    Repo                     ports.EventRepository
    MaxBatch                 int
    ThreatScoreProcessCutoff float64
    Logger                   *slog.Logger
}

// ✅ GOOD: Group config into nested struct
type EventServiceConfig struct {
    MaxBatch                 int
    ThreatScoreProcessCutoff float64
}

type EventServiceOptions struct {
    Repo   ports.EventRepository
    Config EventServiceConfig
    Logger *slog.Logger
}
```

### Required vs Optional Dependencies

```go
func NewSecretService(opts SecretServiceOptions) *SecretService {
    // Required: panic if nil
    if opts.Repo == nil {
        panic("SecretRepository is required")
    }

    // Optional: check for nil before use
    if opts.Logger != nil {
        opts.Logger.Info("SecretService initialized")
    }

    return &SecretService{
        repo:   opts.Repo,
        logger: opts.Logger,
    }
}
```

---

## Testing Patterns

### Unit Tests (with Mocks)

Use gomock for mocking repository interfaces:

```go
func TestSecretService_Create(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockRepo := mocks.NewMockSecretRepository(ctrl)
    svc := NewSecretService(SecretServiceOptions{Repo: mockRepo})

    req := types.CreateSecretRequest{Name: "test", Value: "secret"}
    expected := &types.Secret{ID: "1", Name: "test"}

    mockRepo.EXPECT().Create(gomock.Any(), req).Return(expected, nil)

    got, err := svc.Create(context.Background(), req)
    require.NoError(t, err)
    assert.Equal(t, expected, got)
}
```

### Integration Tests (with Real DB)

Use testutil helpers for integration tests:

```go
func TestSecretService_Create_Integration(t *testing.T) {
    testutil.WithAutoDB(t, func(db *sql.DB) {
        repo := data.NewSecretRepo(db)
        svc := NewSecretService(SecretServiceOptions{Repo: repo})

        req := types.CreateSecretRequest{Name: "test", Value: "secret"}
        got, err := svc.Create(context.Background(), req)

        require.NoError(t, err)
        assert.NotEmpty(t, got.ID)
        assert.Equal(t, "test", got.Name)
    })
}
```

### Test File Naming

- Unit tests: `*_test.go` (same package)
- Integration tests: `*_integration_test.go`
- Workflow tests: `*_workflow_integration_test.go`

---

## Common Patterns

### 1. Simple CRUD Service

For domains with no orchestration logic (most services):

```go
type SecretService struct {
    repo   ports.SecretRepository
    logger *slog.Logger
}

func (s *SecretService) Create(ctx context.Context, req types.CreateSecretRequest) (*types.Secret, error) {
    return s.repo.Create(ctx, req)
}
```

### 2. Orchestration Service

For domains that coordinate multiple repositories:

```go
type SourceService struct {
    sources ports.SourceRepository
    jobs    ports.JobRepository
}

func (s *SourceService) Create(ctx context.Context, req types.CreateSourceRequest) (*types.Source, error) {
    // Step 1: Create source
    source, err := s.sources.Create(ctx, req)
    if err != nil {
        return nil, fmt.Errorf("create source: %w", err)
    }

    // Step 2: Auto-enqueue test job if test source
    if source.Test {
        if err := s.enqueueTestJob(ctx, source); err != nil {
            return nil, fmt.Errorf("enqueue test job: %w", err)
        }
    }

    return source, nil
}
```

### 3. Service with Caching

```go
type SourceService struct {
    sources ports.SourceRepository
    cache   sourceCache  // Optional interface
}

func (s *SourceService) GetByID(ctx context.Context, id string) (*types.Source, error) {
    // Check cache first
    if s.cache != nil {
        if cached, err := s.cache.Get(ctx, id); err == nil {
            return cached, nil
        }
    }

    // Fetch from repository
    source, err := s.sources.GetByID(ctx, id)
    if err != nil {
        return nil, err
    }

    // Cache result (best-effort)
    if s.cache != nil {
        _ = s.cache.Set(ctx, id, source)
    }

    return source, nil
}
```

### 4. Service with Async Operations

```go
type AlertService struct {
    repo       ports.AlertRepository
    dispatcher AlertDispatcher
}

func (s *AlertService) Create(ctx context.Context, req types.CreateAlertRequest) (*types.Alert, error) {
    alert, err := s.repo.Create(ctx, req)
    if err != nil {
        return nil, err
    }

    // Async dispatch (don't block on external calls)
    go func() {
        bgCtx := context.WithoutCancel(ctx)
        if err := s.dispatcher.Dispatch(bgCtx, alert); err != nil {
            s.logger.Error("failed to dispatch alert", "error", err)
        }
    }()

    return alert, nil
}
```

---

## Migration Status

### ✅ Migration Complete (2025-10-02)

All services have been successfully migrated from `internal/core/*_service.go` to `internal/service/` following the Options pattern.

**Services Migrated:**
- ✅ SecretService
- ✅ HTTPAlertSinkService
- ✅ JobService
- ✅ EventService
- ✅ AlertService
- ✅ DomainAllowlistService
- ✅ RuleService
- ✅ SeenDomainService
- ✅ IOCService
- ✅ ProcessedFileService
- ✅ SourceService (consolidated)
- ✅ SiteService
- ✅ AuthService
- ✅ AlertDispatchService
- ✅ AlertSinkService
- ✅ SchedulerService
- ✅ RulesOrchestrationService
- ✅ EventFilterService

**All References Updated:**
- ✅ HTTP handlers
- ✅ Adapters (jobrunner, rulesrunner, scheduler)
- ✅ main.go
- ✅ merrymaker-admin
- ✅ All tests

**See:** [Migration Guide](../../docs/migration-guides/core-to-service-migration.md) for detailed documentation of the migration process.

---

### ℹ️ AllowlistChecker context propagation

- `rules.AllowlistChecker.Allowed` now requires a `context.Context` parameter. Adapters and tests should forward the caller's context instead of constructing a new background context.
- `rules.DomainAllowlistCheckerOptions` gained `FetchTimeout *time.Duration`; leave it `nil` to keep the 10s default or set it to `0` to disable the timeout for long-running lookups.

These changes keep allowlist evaluation within request lifecycles and make timeouts tunable for downstream orchestration layers.

---

## References

- [TEMPLATE.go](./TEMPLATE.go) - Fully documented service template
- [ADR 001: Service Layer Architecture](../../docs/adr/001-service-layer-architecture.md)
- [Service Layer Audit](../../docs/service-layer-audit.md)
- [Hexagonal Architecture](https://alistair.cockburn.us/hexagonal-architecture/)
