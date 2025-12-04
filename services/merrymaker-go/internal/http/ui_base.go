package httpx

import (
	"context"
	"html"
	"log/slog"
	"maps"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/http/ui/viewmodel"
	"github.com/target/mmk-ui-api/internal/service"
)

const errMsgFixBelow = "Please fix the errors below."

// AlertSinksService is a minimal interface for UI needs.
type AlertSinksService interface {
	List(ctx context.Context, limit, offset int) ([]*model.HTTPAlertSink, error)
	GetByID(ctx context.Context, id string) (*model.HTTPAlertSink, error)
	Create(ctx context.Context, req *model.CreateHTTPAlertSinkRequest) (*model.HTTPAlertSink, error)
	Update(ctx context.Context, id string, req *model.UpdateHTTPAlertSinkRequest) (*model.HTTPAlertSink, error)
	Delete(ctx context.Context, id string) (bool, error)
}

// AlertsService is a minimal interface for UI needs.
type AlertsService interface {
	List(ctx context.Context, opts *model.AlertListOptions) ([]*model.Alert, error)
	ListWithSiteNames(ctx context.Context, opts *model.AlertListOptions) ([]*model.AlertWithSiteName, error)
	GetByID(ctx context.Context, id string) (*model.Alert, error)
	Delete(ctx context.Context, id string) (bool, error)
	Resolve(ctx context.Context, params core.ResolveAlertParams) (*model.Alert, error)
}

// Compile-time interface assertions to ensure concrete services satisfy their UI interfaces.
var (
	_ AlertsService           = (*service.AlertService)(nil)
	_ AlertSinksService       = (*service.HTTPAlertSinkService)(nil)
	_ SecretsService          = (*service.SecretService)(nil)
	_ JobReadService          = (*service.JobService)(nil)
	_ EventsService           = (*service.EventService)(nil)
	_ DomainAllowlistsService = (*service.DomainAllowlistService)(nil)
	_ SourcesService          = (*service.SourceService)(nil)
	_ SitesService            = (*service.SiteService)(nil)
)

// SecretsService is a minimal interface for UI needs.
type SecretsService interface {
	List(ctx context.Context, limit, offset int) ([]*model.Secret, error)
	GetByID(ctx context.Context, id string) (*model.Secret, error)
	Create(ctx context.Context, req model.CreateSecretRequest) (*model.Secret, error)
	Update(ctx context.Context, id string, req model.UpdateSecretRequest) (*model.Secret, error)
	Delete(ctx context.Context, id string) (bool, error)
}

// SourcesService is a minimal interface for Sources UI.
type SourcesService interface {
	List(ctx context.Context, limit, offset int) ([]*model.Source, error)
	GetByID(ctx context.Context, id string) (*model.Source, error)
	Create(ctx context.Context, req *model.CreateSourceRequest) (*model.Source, error)
	Delete(ctx context.Context, id string) (bool, error)
	ResolveScript(ctx context.Context, source *model.Source) (string, error)
	// Optional extension methods implemented by the backing repo/service
	CountJobsBySource(ctx context.Context, sourceID string, includeTests bool) (int, error)
	CountBrowserJobsBySource(ctx context.Context, sourceID string, includeTests bool) (int, error)
}

// SitesService is a minimal interface for Sites UI.
type SitesService interface {
	List(ctx context.Context, limit, offset int) ([]*model.Site, error)
	GetByID(ctx context.Context, id string) (*model.Site, error)
	Create(ctx context.Context, req *model.CreateSiteRequest) (*model.Site, error)
	Update(ctx context.Context, id string, req model.UpdateSiteRequest) (*model.Site, error)
	Delete(ctx context.Context, id string) (bool, error)
}

// SitesServiceWithOptions is an optional extension that enables Sites filtering used by the UI via ListWithOptions.
type SitesServiceWithOptions interface {
	SitesService
	ListWithOptions(ctx context.Context, opts model.SitesListOptions) ([]*model.Site, error)
}

// JobReadService exposes job operations needed by the UI.
type JobReadService interface {
	ListRecentByType(ctx context.Context, jobType model.JobType, limit int) ([]*model.Job, error)
	ListRecentByTypeWithSiteNames(
		ctx context.Context,
		jobType model.JobType,
		limit int,
	) ([]*model.JobWithEventCount, error)
	Create(ctx context.Context, req *model.CreateJobRequest) (*model.Job, error)
	GetByID(ctx context.Context, id string) (*model.Job, error)
	Stats(ctx context.Context, jobType model.JobType) (*model.JobStats, error)
	List(ctx context.Context, opts *model.JobListOptions) ([]*model.JobWithEventCount, error)
	Delete(ctx context.Context, id string) error
}

// EventsService is a minimal interface for Events UI.
type EventsService interface {
	ListByJob(ctx context.Context, opts model.EventListByJobOptions) (*model.EventListPage, error)
	CountByJob(ctx context.Context, opts model.EventListByJobOptions) (int, error)
	GetByIDs(ctx context.Context, ids []string) ([]*model.Event, error)
}

// DomainAllowlistsService is a minimal interface for Domain Allowlist UI.
type DomainAllowlistsService interface {
	List(ctx context.Context, opts model.DomainAllowlistListOptions) ([]*model.DomainAllowlist, error)
	GetByID(ctx context.Context, id string) (*model.DomainAllowlist, error)
	Create(ctx context.Context, req *model.CreateDomainAllowlistRequest) (*model.DomainAllowlist, error)
	Update(ctx context.Context, id string, req model.UpdateDomainAllowlistRequest) (*model.DomainAllowlist, error)
	Delete(ctx context.Context, id string) error
	Stats(ctx context.Context, siteID *string) (*model.DomainAllowlistStats, error)
}

// UIHandlers serves browser-facing routes.
type UIHandlers struct {
	T            *TemplateRenderer
	Sinks        AlertSinksService
	AlertsSvc    AlertsService
	Jobs         JobReadService
	JobResults   core.JobResultRepository
	SecretSvc    SecretsService
	SourceSvc    SourcesService
	SiteSvc      SitesService
	EventSvc     EventsService
	AllowlistSvc DomainAllowlistsService
	IOCSvc       IOCService
	IsDev        bool // Development mode flag for enhanced error reporting
	// Optional: to render job results in UI without extra API call
	Orchestrator interface {
		GetJobResults(ctx context.Context, jobID string) (*service.RulesProcessingResults, error)
	}
	Logger *slog.Logger
}

// logger returns the configured logger or falls back to slog.Default().
func (h *UIHandlers) logger() *slog.Logger {
	if h != nil && h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// getPageParams parses pagination params from URL query with sane defaults.
func getPageParams(q url.Values) (int, int) {
	page := 1
	pageSize := 10
	if p := q.Get("page"); p != "" {
		if n, err := strconv.Atoi(p); err == nil && n > 0 {
			page = n
		}
	}
	if s := q.Get("page_size"); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n > 0 && n <= 100 {
			pageSize = n
		}
	}
	return page, pageSize
}

// pageOpts represents pagination options for list views.
type pageOpts struct {
	Page     int
	PageSize int
}

// LimitAndOffset returns limit/offset used for pagination fetches,
// always fetching one extra item to detect next-page availability.
func (p pageOpts) LimitAndOffset() (int, int) {
	page := p.Page
	if page <= 0 {
		page = 1
	}
	pageSize := p.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	limit := pageSize + 1
	offset := (page - 1) * pageSize
	return limit, offset
}

// paginate is a generic paginator for limit/offset list endpoints.
func paginate[T any](
	ctx context.Context,
	p pageOpts,
	fetch func(context.Context, int, int) ([]T, error),
) ([]T, bool, bool, int, int, error) {
	limit, offset := p.LimitAndOffset()
	items, err := fetch(ctx, limit, offset)
	if err != nil {
		return nil, false, false, 0, 0, err
	}
	hasPrev := p.Page > 1
	hasNext := len(items) > p.PageSize
	if hasNext {
		items = items[:p.PageSize]
	}
	if len(items) == 0 {
		return items, hasPrev, hasNext, 0, 0, nil
	}
	startIndex := offset + 1
	endIndex := offset + len(items)
	return items, hasPrev, hasNext, startIndex, endIndex, nil
}

// deleteHandlerOpts encapsulates common delete-handling behavior for UI endpoints.
type deleteHandlerOpts struct {
	ServiceAvailable func() bool
	Delete           func(ctx context.Context, id string) (bool, error)
	RedirectPath     string
	OnError          func(http.ResponseWriter, *http.Request, error)
	OnNotFound       func(http.ResponseWriter, *http.Request)
	OnSuccess        func(http.ResponseWriter, *http.Request, bool)
}

// handleDelete coordinates delete flows shared across UI handlers.
func (h *UIHandlers) handleDelete(w http.ResponseWriter, r *http.Request, opts deleteHandlerOpts) {
	id := r.PathValue("id")
	if id == "" || (opts.ServiceAvailable != nil && !opts.ServiceAvailable()) {
		if opts.OnNotFound != nil {
			opts.OnNotFound(w, r)
		} else {
			h.NotFound(w, r)
		}
		return
	}

	deleted, err := opts.Delete(r.Context(), id)
	if err != nil {
		if opts.OnError != nil {
			opts.OnError(w, r, err)
		} else {
			http.Error(w, "Unable to delete resource.", http.StatusInternalServerError)
		}
		return
	}

	if opts.OnSuccess != nil {
		opts.OnSuccess(w, r, deleted)
		return
	}

	if opts.RedirectPath != "" {
		HTMX(w).Redirect(opts.RedirectPath)
	}
}

// triggerToast sends a standardized HX-Trigger payload for toast notifications.
// Centralizing this avoids repeating the boilerplate map construction across handlers.
func triggerToast(w http.ResponseWriter, message, toastType string) {
	if w == nil || strings.TrimSpace(message) == "" {
		return
	}
	HTMX(w).Trigger("showToast", map[string]any{
		"message": message,
		"type":    strings.TrimSpace(toastType),
	})
}

// FormFrameOpts captures the parameters required to normalize common form data.
type FormFrameOpts struct {
	R           *http.Request
	Data        map[string]any
	DefaultMode FormMode
	MetaForMode func(FormMode) PageMeta
}

// prepareFormFrame normalizes common form rendering fields (Errors, Mode, base layout).
// Returns the hydrated data map and the resolved form mode for further customization.
func prepareFormFrame(opts FormFrameOpts) (map[string]any, FormMode) {
	data := opts.Data
	if data == nil {
		data = map[string]any{}
	}
	if _, ok := data["Errors"]; !ok || data["Errors"] == nil {
		data["Errors"] = map[string]string{}
	}

	mode := resolveFormMode(data["Mode"], opts.DefaultMode)
	data["Mode"] = string(mode)

	if opts.MetaForMode != nil && opts.R != nil {
		maps.Copy(data, basePageData(opts.R, opts.MetaForMode(mode)))
	}

	return data, mode
}

// resolveFormMode coerces assorted Mode representations to a FormMode value.
func resolveFormMode(raw any, fallback FormMode) FormMode {
	switch v := raw.(type) {
	case FormMode:
		if v != "" {
			return v
		}
	case string:
		candidate := FormMode(strings.TrimSpace(v))
		if candidate != "" {
			return candidate
		}
	}
	return fallback
}

// buildPageURL returns a URL with page and page_size set, preserving other query params.
// basePath should be the path without query string (e.g., "/sources", "/alert-sinks").
// Note: This function filters out whitespace-only query parameter values, which standardizes
// behavior across all list views (some previously allowed whitespace-only values).
func buildPageURL(basePath string, q url.Values, p pageOpts) string {
	qq := make(url.Values, len(q))
	for k, v := range q {
		// drop transient/htmx params and empty keys
		if strings.HasPrefix(k, "hx-") || strings.HasPrefix(k, "hx_") {
			continue
		}
		if len(v) == 0 {
			continue
		}
		// filter out empty values while cloning
		tmp := make([]string, 0, len(v))
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				tmp = append(tmp, s)
			}
		}
		if len(tmp) > 0 {
			qq[k] = tmp
		}
	}
	qq.Set("page", strconv.Itoa(p.Page))
	qq.Set("page_size", strconv.Itoa(p.PageSize))
	if enc := qq.Encode(); enc != "" {
		return basePath + "?" + enc
	}
	return basePath
}

// buildCursorURL returns a URL with the supplied cursor param set, preserving filters and page size.
// It also carries forward a best-effort index_offset hint for range displays.
func buildCursorURL(basePath string, q url.Values, cursorParam, cursor string, pageSize, indexOffset int) string {
	qq := make(url.Values, len(q))
	for k, v := range q {
		if strings.HasPrefix(k, "hx-") || strings.HasPrefix(k, "hx_") {
			continue
		}
		if len(v) == 0 {
			continue
		}
		if k == "page" || k == "cursor_after" || k == "cursor_before" || k == "index_offset" {
			continue
		}
		tmp := make([]string, 0, len(v))
		for _, s := range v {
			if strings.TrimSpace(s) != "" {
				tmp = append(tmp, s)
			}
		}
		if len(tmp) > 0 {
			qq[k] = tmp
		}
	}

	if pageSize > 0 {
		qq.Set("page_size", strconv.Itoa(pageSize))
	}
	if indexOffset < 0 {
		indexOffset = 0
	}
	qq.Set("index_offset", strconv.Itoa(indexOffset))
	qq.Set(cursorParam, cursor)

	if enc := qq.Encode(); enc != "" {
		return basePath + "?" + enc
	}
	return basePath
}

// PageMeta contains metadata for page rendering.
type PageMeta struct {
	Title       string
	PageTitle   string
	CurrentPage string
}

// buildLayout constructs shared layout metadata from the request/session context.
func buildLayout(r *http.Request, meta PageMeta) viewmodel.Layout {
	layout := viewmodel.Layout{
		Title:       meta.Title,
		PageTitle:   meta.PageTitle,
		CurrentPage: meta.CurrentPage,
	}

	if csrfToken := GetCSRFToken(r); csrfToken != "" {
		layout.CSRFToken = csrfToken
	}

	if session := GetSessionFromContext(r.Context()); session != nil {
		role := string(session.Role)
		layout.User = &viewmodel.User{
			Email: session.Email,
			Role:  role,
		}
		layout.IsAuthenticated = true
		if role == "admin" {
			layout.CanManageAllowlist = true
			layout.CanManageJobs = true
		}
	}

	return layout
}

// basePageData constructs the common page data map with user context.
func basePageData(r *http.Request, meta PageMeta) map[string]any {
	layout := buildLayout(r, meta)
	data := map[string]any{
		"Title":              layout.Title,
		"PageTitle":          layout.PageTitle,
		"CurrentPage":        layout.CurrentPage,
		"IsAuthenticated":    layout.IsAuthenticated,
		"CanManageAllowlist": layout.CanManageAllowlist,
		"CanManageJobs":      layout.CanManageJobs,
	}

	if layout.CSRFToken != "" {
		data["CSRFToken"] = layout.CSRFToken
	}
	if layout.User != nil {
		data["User"] = layout.User
	}

	return data
}

// PageSpec defines metadata and an optional fetch for page-specific data.
type PageSpec struct {
	Meta  PageMeta
	Fetch func(ctx context.Context, data map[string]any) error
}

// Page builds base data, optionally fetches content data, and renders.
func (h *UIHandlers) Page(w http.ResponseWriter, r *http.Request, spec PageSpec) {
	data := basePageData(r, spec.Meta)
	if err := h.invokePageFetch(r, spec.Fetch, data); err != nil {
		markPageError(data)
	}
	h.renderDashboardPage(w, r, data)
}

// renderDashboardPage renders a dashboard page with proper HTMX partial support.
func (h *UIHandlers) renderDashboardPage(w http.ResponseWriter, r *http.Request, data any) {
	// Handle full page requests first (early return) to reduce nesting
	if !WantsPartial(r) {
		if err := h.T.RenderFull(w, r, data); err != nil {
			h.logAndRenderTemplateError(w, r, err, "full page render")
		}
		return
	}

	// For HTMX requests, render the content plus out-of-band header updates
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	// Hint client JS to update nav active state based on current path
	SetHXTrigger(w, "nav:activate", map[string]string{"path": r.URL.Path})

	layout := extractLayoutInfo(data)

	// Include a <title> element so htmx updates document.title on partial swaps
	safeDocTitle := html.EscapeString(layout.Title)
	if _, err := w.Write([]byte(`<title>` + safeDocTitle + `</title>`)); err != nil {
		h.logger().Error("failed to write partial document title", "error", err)
		return
	}

	// Out-of-band update for the header title
	safeTitle := html.EscapeString(layout.PageTitle)
	if _, err := w.Write([]byte(`<h1 id="header-title" class="header-title" hx-swap-oob="outerHTML">` + safeTitle + `</h1>`)); err != nil {
		h.logger().Error("failed to write partial header title", "error", err)
		return
	}

	if err := h.T.t.ExecuteTemplate(w, ContentTemplateFor(layout.CurrentPage), data); err != nil {
		h.logAndRenderTemplateError(w, r, err, "partial content render")
		return
	}
}

func (h *UIHandlers) invokePageFetch(
	r *http.Request,
	fetchFn func(ctx context.Context, data map[string]any) error,
	data map[string]any,
) error {
	if fetchFn == nil {
		return nil
	}
	return fetchFn(r.Context(), data)
}

func markPageError(data map[string]any) {
	data["Error"] = true
	if _, ok := data["ErrorMessage"]; ok {
		return
	}
	data["ErrorMessage"] = "An unexpected error occurred. Please try again."
}

func layoutFromProvider(data any) *viewmodel.Layout {
	provider, ok := data.(viewmodel.LayoutProvider)
	if !ok {
		return nil
	}
	return provider.LayoutData()
}

func layoutFromPointer(data any) *viewmodel.Layout {
	layout, ok := data.(*viewmodel.Layout)
	if !ok || layout == nil {
		return nil
	}
	return layout
}

func layoutFromMap(data any) viewmodel.Layout {
	m, mapOK := data.(map[string]any)
	if !mapOK {
		return viewmodel.Layout{}
	}

	layout := viewmodel.Layout{}
	if v, titleOK := m["Title"].(string); titleOK {
		layout.Title = v
	}
	if v, pageTitleOK := m["PageTitle"].(string); pageTitleOK {
		layout.PageTitle = v
	}
	if v, currentPageOK := m["CurrentPage"].(string); currentPageOK {
		layout.CurrentPage = v
	}
	return layout
}

func extractLayoutInfo(data any) viewmodel.Layout {
	if layout := layoutFromProvider(data); layout != nil {
		return *layout
	}

	if layout, ok := data.(viewmodel.Layout); ok {
		return layout
	}

	if layout := layoutFromPointer(data); layout != nil {
		return *layout
	}

	return layoutFromMap(data)
}

// logAndRenderTemplateError logs template errors and renders them in dev mode.
func (h *UIHandlers) logAndRenderTemplateError(w http.ResponseWriter, r *http.Request, err error, context string) {
	h.logger().Error("template rendering failed",
		"error", err,
		"context", context,
		"path", r.URL.Path,
		"method", r.Method,
	)

	// In dev mode, show detailed error in the response
	if h.IsDev {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.WriteHeader(http.StatusInternalServerError)
		errHTML := html.EscapeString(err.Error())
		pathHTML := html.EscapeString(r.URL.Path)
		contextHTML := html.EscapeString(context)
		if _, writeErr := w.Write([]byte(`
			<div style="padding: 20px; background: #fee; border: 2px solid #c33; border-radius: 4px; margin: 20px; font-family: monospace;">
				<h2 style="color: #c33; margin-top: 0;">Template Rendering Error</h2>
				<p><strong>Context:</strong> ` + contextHTML + `</p>
				<p><strong>Path:</strong> ` + pathHTML + `</p>
				<p><strong>Error:</strong></p>
				<pre style="background: #fff; padding: 10px; border: 1px solid #ccc; overflow-x: auto;">` + errHTML + `</pre>
			</div>
		`)); writeErr != nil {
			h.logger().Error("failed to write template error response", "error", writeErr)
		}
		return
	}

	// In production, show generic error
	http.Error(w, "internal server error", http.StatusInternalServerError)
}
