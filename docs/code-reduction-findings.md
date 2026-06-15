# Code Reduction Findings

> **Generated:** 2026-05-12  
> **Scope:** Full codebase inspection of `mmk-ui-api` (Go service + TypeScript puppeteer-worker)  
> **Method:** Indexed symbol search, flow tracing, reference analysis, pattern matching  
> **Goal:** Identify dead code, duplicate code flows, deep nesting, and reduction opportunities without breaking the codebase

---

## Executive Summary

| Category | Count | Impact | Risk |
|----------|-------|--------|------|
| Dead Code (safe to remove) | 5 items | Medium | Low |
| Duplicate Code Patterns | 6 patterns | High | Low-Medium |
| Deep Call Chains (4+ levels) | 3 chains | Medium | Medium |
| Over-Engineering | 2 areas | Low | Low |

**Estimated reduction:** ~15-25% of HTTP layer code, ~5 dead files/functions removable immediately.

---

## 1. DEAD CODE (Safe to Remove)

### 1.1 `internal/core/alert_adapter.go` — Deprecated AlertService

**File:** `services/merrymaker-go/internal/core/alert_adapter.go`  
**Lines:** 1–199 (entire file)  
**Evidence:** Zero indexed references. File header explicitly says:
```go
// Deprecated: Use service.AlertService (internal/service/alert.go) instead.
// This file is kept only for backward compatibility during migration.
// Will be removed once all references are migrated.
```

**Recommendation:** **DELETE ENTIRE FILE.** The replacement `service.AlertService` in `internal/service/alert.go` is the active implementation.

### 1.2 `internal/core/alert_dispatcher.go` — Duplicate AlertDispatcher Interface

**File:** `services/merrymaker-go/internal/core/alert_dispatcher.go`  
**Lines:** 1–60 (entire file)  
**Evidence:** Zero indexed references. The `AlertDispatcher` interface is defined in TWO places:
- `internal/core/alert_dispatcher.go` (DEAD — never used)
- `internal/service/alert.go` (LIVE — used by `rulesrunner`, `alert_test.go`, `bootstrap`)

**Recommendation:** **DELETE ENTIRE FILE.** The live version lives in `internal/service/alert.go`.

### 1.3 `internal/service/event_filter.go` — Test-Only Methods

**File:** `services/merrymaker-go/internal/service/event_filter.go`  
**Methods:** `AddProcessableEventType()`, `RemoveProcessableEventType()`, `SetProcessableEventType()`, `FilterProcessableEvents()`  
**Evidence:** All four methods are referenced **only in test files** (`event_filter_test.go`). No production code calls them.

The filter service is used in production via `ShouldProcessEvents()` and `ShouldProcessEvent()` only.

**Recommendation:** Either:
- **Option A:** Remove the four test-only methods (update tests accordingly)
- **Option B:** Keep them but mark with `// For testing only` comment to signal intent

### 1.4 `services/puppeteer-worker/src/examples.ts` — Demo Code in Production Export

**File:** `services/puppeteer-worker/src/examples.ts`  
**Evidence:** Re-exported via `index.ts` (`export * as Examples from "./examples.js"`) but has **zero internal callers**. Only accessible to external consumers.

**Recommendation:** Move to `examples/` directory or exclude from production bundle. Currently ships demo code in the main package.

### 1.5 `services/merrymaker-go/internal/service/TEMPLATE.go` — Documentation Template

**File:** `services/merrymaker-go/internal/service/TEMPLATE.go`  
**Evidence:** Has `//go:build ignore` directive — not compiled. 300+ lines of documentation template.

**Recommendation:** Move to `docs/` or `docs/service-pattern.md`. It doesn't break anything but is misplaced in a source directory.

---

## 2. DUPLICATE CODE PATTERNS

### 2.1 Template Render Helpers (HIGH IMPACT)

**Files:**
- `internal/http/templates/core/funcs.go` (lines 42–64: `addRenderFuncs`)
- `internal/http/templates/events/funcs.go` (lines 30–70: inline `renderEventPartial`)

**Duplication:** Both implement the same pattern:
```go
// core/funcs.go — renderSection
funcs["renderSection"] = func(page string, data any) (template.HTML, error) {
    if *deps.Template == nil { return "", errors.New("template not initialized") }
    var buf bytes.Buffer
    if err := deps.Template.ExecuteTemplate(&buf, page, data); err != nil {
        return "", err
    }
    // G203: ... security comment
    return template.HTML(buf.String()), nil
}

// events/funcs.go — renderEventPartial (NEARLY IDENTICAL)
funcs["renderEventPartial"] = func(name string, data any) (template.HTML, error) {
    if *deps.Template == nil { return "", errors.New("template not initialized") }
    var buf bytes.Buffer
    if err := deps.Template.ExecuteTemplate(&buf, name, data); err != nil {
        return "", err
    }
    // G203: ... security comment
    return template.HTML(buf.String()), nil
}
```

**Also duplicated:** `toJSON` (core) vs `serializeEventsToJSON` (events) — both do `json.Marshal` + `template.JS` wrapping with identical security comments.

**Recommendation:** Create `internal/http/templates/shared/render.go`:
```go
// shared/render.go
func RenderPartial(t **template.Template) func(string, any) (template.HTML, error) {
    return func(name string, data any) (template.HTML, error) { ... }
}
func SafeMarshalToJS(v any) (template.JS, error) { ... }
```

**Savings:** ~40 lines of duplicated code, single source of truth for security comment.

### 2.2 `build*Options` Functions (HIGH IMPACT)

**Files:**
- `internal/http/ui_sites_form.go` (lines 20–59)
- `internal/http/ui_alert_sinks_form.go` (lines 132–154)

**Duplication:** Three functions with identical structure:
```go
// buildSourceOptions (ui_sites_form.go:20-38)
func (h *UIHandlers) buildSourceOptions(ctx context.Context, selectedID string) ([]map[string]any, error) {
    list, err := h.SourceSvc.List(ctx, optionListLimit, 0)
    sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
    for _, s := range list {
        out = append(out, map[string]any{"ID": s.ID, "Name": s.Name, "Selected": s.ID == selectedID})
    }
}

// buildAlertSinkOptions (ui_sites_form.go:41-59) — NEARLY IDENTICAL
func (h *UIHandlers) buildAlertSinkOptions(ctx context.Context, selectedID string) ([]map[string]any, error) {
    list, err := h.Sinks.List(ctx, optionListLimit, 0)
    sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
    for _, s := range list {
        out = append(out, map[string]any{"ID": s.ID, "Name": s.Name, "Selected": s.ID == selectedID})
    }
}

// buildSecretOptions (ui_alert_sinks_form.go:132-154) — SIMILAR PATTERN
func (h *UIHandlers) buildSecretOptions(ctx context.Context, selected []string) []map[string]any {
    list, err := h.SecretSvc.List(ctx, 1000, 0)
    sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
    for _, s := range list {
        out = append(out, map[string]any{"Name": s.Name, "Selected": sel})
    }
}
```

**Recommendation:** Create a generic helper in `ui_base.go`:
```go
func buildSelectOptions[T any](
    ctx context.Context,
    listFn func(context.Context, int, int) ([]T, error),
    selected any,
    nameFn func(T) string,
    idFn func(T) string,
    limit int,
) ([]map[string]any, error)
```

**Savings:** ~60 lines → ~10 lines of generic code + 3 one-line callers.

### 2.3 UI List Handlers (MEDIUM IMPACT)

**Files:**
- `internal/http/ui_iocs.go` (lines 17–140)
- `internal/http/ui_sources.go` (lines 17–140)
- `internal/http/ui_sites.go` (lines 17–180)

**Duplication:** All three follow identical structure:
1. Define `xFilter` struct (Category, Search, Status, SortBy, SortOrder, Page, PageSize)
2. Define `parseXFilter(r *http.Request) xFilter` function
3. Define `HandleListX(w, r)` → `handleList()` → `enrichXData()`
4. Enrich function calls `basePageData()` + `h.XSvc.List()`

The filter structs differ only in which categories are valid. The parse/enrich pattern is identical.

**Recommendation:** Already partially handled by `ListHandlerOpts` generic. Could further consolidate filter parsing into a generic `parseFilter[FilterType]()` with a `ValidCategories() []string` interface on filter types.

**Savings:** ~50 lines per file could be reduced to ~20 lines with generic filter parsing.

### 2.4 Form Frame Preparation (MEDIUM IMPACT)

**Files:** Multiple `ui_*_form.go` files  
**Pattern:** Every form handler calls `prepareFormFrame()` + `renderDashboardPage()` with slightly different PageMeta.

**Current state:** Already well-consolidated via `prepareFormFrame(FormFrameOpts)` in `ui_base.go`. The domain-specific render functions (`renderSiteForm`, `renderSourceForm`, `renderAlertSinkFormWithData`) are intentionally different per the code comments.

**Assessment:** **No action needed.** This is already well-factored.

### 2.5 Three `Funcs()` Functions Across Template Packages (LOW IMPACT)

**Files:**
- `internal/http/templates/assets/funcs.go` — `Funcs(opts Options)`
- `internal/http/templates/core/funcs.go` — `Funcs(deps Deps)`
- `internal/http/templates/events/funcs.go` — `Funcs(deps Deps)`

**Duplication:** Each constructs a `template.FuncMap` with its own helpers. The plumbing (creating map, registering helpers) is identical.

**Recommendation:** Create a `template.FuncMapBuilder` that chains registrations:
```go
builder := NewFuncMapBuilder(deps)
builder.Register(core.Funcs(deps))
builder.Register(events.Funcs(deps))
builder.Register(assets.Funcs(opts))
finalMap := builder.Build()
```

**Savings:** Minor (~10 lines of plumbing per caller).

---

## 3. DEEP CALL CHAINS (4+ Levels)

### 3.1 Job Execution Chain (5 levels deep)

```
cmd/merrymaker/main.go:run()
  → bootstrap.initInfrastructure()
    → scheduler.Run()
      → rulesrunner.Run()
        → rules.Coordinator.Run()
          → rules.DefaultPipeline.Run()
            → rules.Extractor.Extract()
              → builtin_rules (UnknownDomainRule, IOCRule, etc.)
```

**Depth:** 7 levels from `main.run()` to actual rule execution.

**Recommendation:** This is architectural (clean architecture layers) — not easily flattenable without violating separation of concerns. **Accept as-is.**

### 3.2 Puppeteer Worker Chain (5 levels deep)

```
worker-main.ts:main()
  → WorkerLoop.start()
    → WorkerLoop.executeJob()
      → PuppeteerRunner.runScript()
        → PuppeteerRunner.initialize()
          → EventMonitor.attach() + FileCapture + EventShipper
```

**Depth:** 5 levels, with `PuppeteerRunner` being the complexity hotspot (~400+ lines).

**Recommendation:** Split `PuppeteerRunner.runScript()` into smaller composable methods:
- `launchBrowser()`
- `attachMonitoring()`
- `executeScript()`
- `collectAndShipEvents()`
- `cleanup()`

This doesn't reduce total code but reduces cognitive complexity and nesting depth within `runScript()`.

### 3.3 HTTP Handler → Service → Repository Chain (4-5 levels)

```
UIHandlers.HandleListSources()
  → handleList(ListHandlerOpts)
    → sourceService.List()
      → sourceRepo.List()
        → db.Query()
```

**Depth:** 5 levels for a simple list page.

**Recommendation:** This is standard clean architecture. The `handleList()` generic handler already reduces duplication. **Accept as-is.**

---

## 4. OVER-ENGINEERING

### 4.1 `EventFilterService` — Singleton With Mutable State

**File:** `internal/service/event_filter.go`  
**Issue:** Uses a package-level `defaultProcessableEventTypes` variable with `sync.RWMutex` for thread-safe mutation. This is a global mutable singleton.

**Problem:**
- Makes testing harder (state leaks between tests)
- The `AddProcessableEventType`/`RemoveProcessableEventType` methods (dead in production) suggest this was designed for runtime reconfiguration that never happened
- Only `ShouldProcessEvent()` and `ShouldProcessEvents()` are used in production — both read-only

**Recommendation:** Convert to an immutable, constructor-configured service:
```go
type EventFilterService struct {
    processableTypes map[string]bool
}

func NewEventFilterService(types []string) *EventFilterService {
    m := make(map[string]bool)
    for _, t := range types { m[t] = true }
    return &EventFilterService{processableTypes: m}
}
```

**Savings:** ~30 lines (remove mutex, remove mutation methods, remove global state).

### 4.2 `JobRepositoryTx` Interface — Optional Transactional Pattern

**File:** `internal/core/interfaces.go` (lines 30–35)  
**Issue:** Defines an optional transactional interface that only ONE caller uses via type assertion:
```go
if creator, ok := s.jobs.(core.JobRepositoryTx); ok {
    // use transactional path
}
```

**Assessment:** This is a legitimate pattern for optional capabilities. **Keep as-is** but document the type-assertion pattern.

---

## 5. QUICK WINS (Low Risk, High Impact)

| # | Action | File(s) | Lines Saved | Risk |
|---|--------|---------|-------------|------|
| Q1 | Delete `core/alert_adapter.go` | 1 file | ~199 | None |
| Q2 | Delete `core/alert_dispatcher.go` | 1 file | ~60 | None |
| Q3 | Move `TEMPLATE.go` to `docs/` | 1 file | 0 (relocate) | None |
| Q4 | Consolidate `build*Options` into generic | 3 functions → 1 | ~50 | Low |
| Q5 | Extract shared render helper for templates | 2 files → 1 shared | ~40 | Low |
| Q6 | Remove test-only methods from `EventFilterService` | 4 methods | ~30 | Low |
| Q7 | Move `examples.ts` out of production export | 1 file | 0 (relocate) | None |

---

## 6. DETAILED FILE INVENTORY

### Go Service (`merrymaker-go`) — Files by Category

**Dead (safe to delete):**
- `internal/core/alert_adapter.go` — 199 lines
- `internal/core/alert_dispatcher.go` — 60 lines

**Misplaced (relocate):**
- `internal/service/TEMPLATE.go` — 300+ lines → move to `docs/`

**Duplication hotspots:**
- `internal/http/ui_sites_form.go` — `buildSourceOptions`/`buildAlertSinkOptions` (identical)
- `internal/http/ui_alert_sinks_form.go` — `buildSecretOptions` (similar pattern)
- `internal/http/templates/core/funcs.go` — render helper duplicated in events
- `internal/http/templates/events/funcs.go` — render helper duplicated in core

**Over-engineered:**
- `internal/service/event_filter.go` — global mutable state, test-only methods

### TypeScript Worker (`puppeteer-worker`)

**Dead (safe to relocate):**
- `src/examples.ts` — demo code in production export

**Complexity hotspot:**
- `src/puppeteer-runner.ts` — 400+ lines, 5-level call chain

**Well-factored (no action needed):**
- `src/logger.ts` vs `src/lib/logger.ts` — complementary, not duplicated
- `src/config-loader.ts` vs `src/config-schema.ts` — layered, not duplicated
- `src/event-monitor.ts` vs `src/event-shipper.ts` — separate responsibilities

---

## 7. RECOMMENDED IMPLEMENTATION ORDER

### Phase 1: Zero-Risk Cleanup (Day 1)
1. Delete `internal/core/alert_adapter.go`
2. Delete `internal/core/alert_dispatcher.go`
3. Move `internal/service/TEMPLATE.go` → `docs/service-pattern.md`
4. Move `services/puppeteer-worker/src/examples.ts` → `services/puppeteer-worker/examples/`
5. Update `index.ts` to remove examples export

### Phase 2: Duplication Reduction (Day 2-3)
6. Create `internal/http/templates/shared/render.go` with shared render helper
7. Refactor `core/funcs.go` and `events/funcs.go` to use shared helper
8. Create generic `buildSelectOptions()` in `ui_base.go`
9. Replace `buildSourceOptions`, `buildAlertSinkOptions`, `buildSecretOptions` with generic calls

### Phase 3: Complexity Reduction (Day 4-5)
10. Refactor `EventFilterService` to immutable constructor pattern
11. Split `PuppeteerRunner.runScript()` into smaller methods
12. Review and consolidate filter parsing across UI list handlers

---

## 8. WHAT NOT TO TOUCH

- **`internal/http/form_handler.go`** — Generic form handler is well-designed
- **`internal/http/list_handler.go`** — Generic list handler is well-designed
- **`internal/http/error_renderer.go`** — Error rendering is well-factored
- **`internal/http/validation/validators.go`** — Validator chain pattern is idiomatic
- **`internal/domain/rules/`** — Rules engine complexity is justified by domain
- **`internal/bootstrap/`** — Dependency wiring is clear and well-structured
- **`internal/http/middleware.go`** — Middleware chain is standard and well-tested
