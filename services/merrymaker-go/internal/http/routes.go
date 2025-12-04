package httpx

import (
	"bytes"
	"io/fs"
	"log"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	merrymaker "github.com/target/mmk-ui-api"
	"github.com/target/mmk-ui-api/internal/core"
	domainauth "github.com/target/mmk-ui-api/internal/domain/auth"
	"github.com/target/mmk-ui-api/internal/service"
)

// RouterServices holds all the services needed by the HTTP router.
type RouterServices struct {
	Jobs           *service.JobService
	Events         *service.EventService
	Secrets        *service.SecretService
	HTTPAlertSinks *service.HTTPAlertSinkService
	Alerts         AlertsService
	Sources        *service.SourceService
	Sites          *service.SiteService
	Allowlist      *service.DomainAllowlistService
	IOC            *service.IOCService
	Auth           *service.AuthService
	CookieDomain   string
	// Optional: rules orchestration (for deduped enqueues)
	Orchestrator *service.RulesOrchestrationService
	// Optional: Event filter service (DI-friendly). If nil, a default will be created.
	Filter     *service.EventFilterService
	JobResults core.JobResultRepository
	// Configuration
	AutoEnqueueRules bool
	IsDev            bool         // Development mode flag for hot reloading, etc.
	Logger           *slog.Logger // Logger for template and HTTP errors (optional)
}

// NewRouter creates and configures a new HTTP router with browser middleware.
func NewRouter(services RouterServices) http.Handler {
	mux := http.NewServeMux()

	jobHandlers := &JobHandlers{Svc: services.Jobs, Orchestrator: services.Orchestrator}
	filterSvc := services.Filter
	if filterSvc == nil {
		filterSvc = service.NewEventFilterService()
	}
	eventHandlers := NewEventHandlers(EventHandlersOptions{
		EventService:     services.Events,
		FilterService:    filterSvc,
		JobService:       services.Jobs,
		Orchestrator:     services.Orchestrator,
		SiteService:      services.Sites,
		AutoEnqueueRules: services.AutoEnqueueRules,
		Logger:           services.Logger,
	})
	secretHandlers := &SecretHandlers{Svc: services.Secrets}
	alertHandlers := &HTTPAlertSinkHandlers{Svc: services.HTTPAlertSinks}
	sourceHandlers := &SourceHandlers{Svc: services.Sources}
	var authHandlers *AuthHandlers
	if services.Auth != nil {
		authHandlers = &AuthHandlers{Svc: services.Auth, CookieDomain: services.CookieDomain, Logger: services.Logger}
	}

	registerJobRoutes(mux, jobHandlers)
	registerEventRoutes(mux, eventHandlers)
	registerSecretRoutes(mux, secretHandlers, services.Auth)
	registerAlertSinkRoutes(mux, alertHandlers, services.Auth)
	registerIOCRoutes(mux, &IOCHandlers{Svc: services.IOC}, services.Auth)
	registerSourceRoutes(mux, sourceHandlers)
	mux.Handle("GET /healthz", http.HandlerFunc(healthHandler))
	mux.Handle("HEAD /healthz", http.HandlerFunc(healthHandler))
	if authHandlers != nil {
		registerAuthRoutes(mux, authHandlers)
	}

	// Static assets at /static
	// Dev mode: serve from disk for hot reloading
	// Prod mode: serve from embedded FS
	mux.Handle("GET /static/", staticWithFallback(services.IsDev))

	// UI routes with template renderer
	uiHandlers := setupUIHandlers(services)
	if uiHandlers != nil {
		cfg := uiRouteConfig{Auth: services.Auth, CookieDomain: services.CookieDomain}
		registerUIRoutes(mux, uiHandlers, cfg)
	}

	// Wrap with NotFound handler and browser detection middleware
	handler := &notFoundHandler{
		mux:        mux,
		uiHandlers: uiHandlers,
	}

	// Apply browser detection middleware
	return BrowserDetection()(handler)
}

// setupDevMode configures template FS, critical CSS FS, and asset resolver for dev mode.
func setupDevMode(diskManifestPath string) (fs.FS, fs.FS, *AssetResolver) {
	templateFS := os.DirFS(TemplatePathFromRoot)
	criticalCSSFS := os.DirFS("frontend/public")

	resolver, err := NewAssetResolverFromDisk(diskManifestPath)
	if err != nil {
		log.Printf(
			"failed to load asset manifest %s: %v; falling back to logical asset names",
			diskManifestPath,
			err,
		)
	}
	return templateFS, criticalCSSFS, resolver
}

// setupProdMode configures template FS, critical CSS FS, and asset resolver for production mode.
func setupProdMode(diskManifestPath string) (fs.FS, fs.FS, *AssetResolver) {
	templateFS, err := fs.Sub(merrymaker.TemplateFS, "frontend/templates")
	if err != nil {
		log.Printf("failed to create sub-filesystem for templates: %v; falling back to disk", err)
		templateFS = os.DirFS(TemplatePathFromRoot)
	}

	criticalCSSFS, resolver := setupProdAssets(diskManifestPath)
	return templateFS, criticalCSSFS, resolver
}

// setupProdAssets configures critical CSS FS and asset resolver for production mode.
func setupProdAssets(diskManifestPath string) (fs.FS, *AssetResolver) {
	staticSub, err := fs.Sub(merrymaker.StaticFS, "frontend/static")
	if err != nil {
		log.Printf("failed to create sub-filesystem for static assets: %v", err)
		return nil, tryDiskManifest(diskManifestPath)
	}

	resolver, err := NewAssetResolverFromFS(staticSub, "manifest.json")
	if err != nil {
		log.Printf("failed to load asset manifest from embedded FS: %v", err)
		return staticSub, tryDiskManifest(diskManifestPath)
	}

	return staticSub, resolver
}

// tryDiskManifest attempts to load the asset manifest from disk as a fallback.
func tryDiskManifest(diskManifestPath string) *AssetResolver {
	resolver, err := NewAssetResolverFromDisk(diskManifestPath)
	if err != nil {
		log.Printf(
			"failed to load asset manifest %s: %v; falling back to logical asset names",
			diskManifestPath,
			err,
		)
	}
	return resolver
}

// setupUIHandlers creates UI handlers with template renderer and asset resolver.
// In dev mode (services.IsDev=true), templates are loaded from disk for hot reloading.
// In production mode (services.IsDev=false), templates are loaded from embedded FS.
func setupUIHandlers(services RouterServices) *UIHandlers {
	// Choose template filesystem based on dev mode
	var templateFS fs.FS
	var criticalCSSFS fs.FS
	var resolver *AssetResolver

	diskManifestPath := filepath.Join("frontend", "static", "manifest.json")

	if services.IsDev {
		templateFS, criticalCSSFS, resolver = setupDevMode(diskManifestPath)
	} else {
		templateFS, criticalCSSFS, resolver = setupProdMode(diskManifestPath)
	}

	if resolver == nil {
		resolver = &AssetResolver{}
	}

	tr, err := NewTemplateRenderer(TemplateRendererConfig{
		TemplateFS:    templateFS,
		Resolver:      resolver,
		CriticalCSSFS: criticalCSSFS,
		DevMode:       services.IsDev,
		Logger:        services.Logger,
	})
	if err != nil {
		if services.Logger != nil {
			services.Logger.Error("failed to create template renderer", slog.Any("error", err))
		} else {
			log.Printf("ERROR: failed to create template renderer: %v", err)
		}
		return nil
	}

	return &UIHandlers{
		T:            tr,
		Sinks:        services.HTTPAlertSinks,
		AlertsSvc:    services.Alerts,
		Jobs:         services.Jobs,
		JobResults:   services.JobResults,
		SecretSvc:    services.Secrets,
		SourceSvc:    services.Sources,
		SiteSvc:      services.Sites,
		EventSvc:     services.Events,
		AllowlistSvc: services.Allowlist,
		IOCSvc:       services.IOC,
		Orchestrator: services.Orchestrator,
		IsDev:        services.IsDev,
		Logger:       services.Logger,
	}
}

// staticWithFallback serves /static/* assets.
// In dev mode (isDev=true), serves from disk with fallback for hot reloading.
// In production mode (isDev=false), serves from embedded FS.
func staticWithFallback(isDev bool) http.Handler {
	if isDev {
		// Dev mode: serve from disk with fallback for hot reloading
		mfs := multiFS{
			http.Dir("frontend/static"),
			http.Dir("frontend/public"),
			devCSSFS{},
		}
		return staticWithCacheHeaders(http.StripPrefix("/static/", http.FileServer(mfs)))
	}

	// Production mode: serve from embedded FS
	staticSub, err := fs.Sub(merrymaker.StaticFS, "frontend/static")
	if err != nil {
		log.Printf("failed to create sub-filesystem for static assets: %v", err)
		// Fallback to disk serving if embed fails
		return staticWithCacheHeaders(http.StripPrefix("/static/", http.FileServer(http.Dir("frontend/static"))))
	}
	return staticWithCacheHeaders(http.StripPrefix("/static/", http.FileServer(http.FS(staticSub))))
}

// multiFS provides fallback filesystem for dev mode.
type multiFS []http.FileSystem

func (m multiFS) Open(name string) (http.File, error) {
	for _, fsys := range m {
		f, err := fsys.Open(name)
		if err == nil {
			return f, nil
		}
		// ignore not-exist and try next, but return early on other errors
		if !os.IsNotExist(err) {
			return nil, err
		}
	}
	return nil, os.ErrNotExist
}

// devCSSFS maps a single CSS path used during dev to the source stylesheet.
type devCSSFS struct{}

func (devCSSFS) Open(name string) (http.File, error) {
	if strings.TrimPrefix(name, "/") == "css/styles.css" || name == "css/styles.css" {
		return os.Open("frontend/styles/index.css")
	}
	return nil, os.ErrNotExist
}

// staticWithCacheHeaders wraps a static file handler to add appropriate cache headers.
func staticWithCacheHeaders(handler http.Handler) http.Handler {
	// Regex to match content-hashed filenames including optional .map (e.g., app.abc123.js, styles.def456.css, app.abc123.js.map)
	hashedFilePattern := regexp.MustCompile(`\.[a-f0-9]{8}\.(?:js|css)(?:\.map)?$`)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if this is a content-hashed asset
		if hashedFilePattern.MatchString(r.URL.Path) {
			// Hashed assets can be cached for a long time (1 year)
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			// Non-hashed assets (dev mode) should not be cached
			w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
			w.Header().Set("Pragma", "no-cache")
			w.Header().Set("Expires", "0")
		}

		handler.ServeHTTP(w, r)
	})
}

// notFoundHandler wraps a ServeMux and provides custom 404 handling.
type notFoundHandler struct {
	mux        *http.ServeMux
	uiHandlers *UIHandlers
}

// ServeHTTP implements http.Handler and provides custom 404 handling.
func (h *notFoundHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	cw := newCaptureWriter(w)
	// Serve the request through the mux, capturing status, headers, and body
	h.mux.ServeHTTP(cw, r)

	// If the mux didn't handle the request (404), use our custom handler
	if cw.status == http.StatusNotFound {
		// For missing static assets, preserve the default file server response
		if strings.HasPrefix(r.URL.Path, "/static/") {
			cw.flushTo(w)
			return
		}
		if h.uiHandlers != nil {
			h.uiHandlers.NotFound(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}

	// Not a 404: write the captured response
	cw.flushTo(w)
}

// captureWriter buffers headers, status and body so we can decide post-dispatch.
type captureWriter struct {
	rw     http.ResponseWriter
	header http.Header
	status int
	buf    bytes.Buffer
}

func newCaptureWriter(w http.ResponseWriter) *captureWriter {
	return &captureWriter{rw: w, header: make(http.Header), status: http.StatusOK}
}

func (c *captureWriter) Header() http.Header         { return c.header }
func (c *captureWriter) WriteHeader(code int)        { c.status = code }
func (c *captureWriter) Write(b []byte) (int, error) { return c.buf.Write(b) }

func (c *captureWriter) flushTo(w http.ResponseWriter) {
	for k, vs := range c.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(c.status)
	if _, err := w.Write(c.buf.Bytes()); err != nil {
		log.Printf("failed to write captured response: %v", err)
	}
}

func registerJobRoutes(mux *http.ServeMux, h *JobHandlers) {
	mux.HandleFunc("POST /api/jobs", h.CreateJob)
	mux.HandleFunc("GET /api/jobs/{type}/reserve_next", h.ReserveNext)
	mux.HandleFunc("GET /api/jobs/{type}/stats", h.Stats)
	mux.HandleFunc("GET /api/jobs/{id}/status", h.GetStatus)
	mux.HandleFunc("GET /api/jobs/{id}/rules-results", h.GetRulesResults)
	mux.HandleFunc("POST /api/jobs/{id}/heartbeat", h.Heartbeat)
	mux.HandleFunc("POST /api/jobs/{id}/complete", h.Complete)
	mux.HandleFunc("POST /api/jobs/{id}/fail", h.Fail)
}

func registerEventRoutes(mux *http.ServeMux, h *EventHandlers) {
	mux.HandleFunc("POST /api/events/bulk", h.BulkInsert)
	mux.HandleFunc("GET /api/jobs/{id}/events", h.ListByJob)
}

func registerSecretRoutes(mux *http.ServeMux, h *SecretHandlers, auth *service.AuthService) {
	registerCRUD(mux, crudRoutes{
		Base:    "/api/secrets",
		Create:  h.Create,
		List:    h.List,
		GetByID: h.GetByID,
		Update:  h.Update,
		Delete:  h.Delete,
		Middleware: func(hh http.Handler) http.Handler {
			if auth != nil {
				return RequireRole(auth, domainauth.RoleAdmin)(hh)
			}
			return hh
		},
	})
}

func registerAlertSinkRoutes(
	mux *http.ServeMux,
	h *HTTPAlertSinkHandlers,
	auth *service.AuthService,
) {
	registerCRUD(mux, crudRoutes{
		Base:    "/api/http-alert-sinks",
		Create:  h.Create,
		List:    h.List,
		GetByID: h.GetByID,
		Update:  h.Update,
		Delete:  h.Delete,
		Middleware: func(hh http.Handler) http.Handler {
			if auth != nil {
				return RequireRole(auth, domainauth.RoleAdmin)(hh)
			}
			return hh
		},
	})
}

func registerIOCRoutes(mux *http.ServeMux, h *IOCHandlers, auth *service.AuthService) {
	// Nil-safe middleware factory
	adminOnly := func(hh http.Handler) http.Handler {
		if auth != nil {
			return RequireRole(auth, domainauth.RoleAdmin)(hh)
		}
		return hh
	}

	registerCRUD(mux, crudRoutes{
		Base:       "/api/iocs",
		Create:     h.Create,
		List:       h.List,
		GetByID:    h.GetByID,
		Update:     h.Update,
		Delete:     h.Delete,
		Middleware: adminOnly,
	})

	// Additional IOC-specific endpoints (use same nil-safe middleware)
	mux.Handle("POST /api/iocs/bulk", adminOnly(http.HandlerFunc(h.BulkCreate)))
	mux.Handle("GET /api/iocs/stats", adminOnly(http.HandlerFunc(h.Stats)))
}

// registerCRUD registers standard CRUD routes for a resource base path, applying mw if non-nil.
type crudRoutes struct {
	Base       string
	Create     http.HandlerFunc
	List       http.HandlerFunc
	GetByID    http.HandlerFunc
	Update     http.HandlerFunc
	Delete     http.HandlerFunc
	Middleware func(http.Handler) http.Handler
}

func registerCRUD(mux *http.ServeMux, cfg crudRoutes) {
	if cfg.Base == "" {
		panic("registerCRUD: Base must not be empty") //nolint:forbidigo // Fail fast during server setup.
	}
	if cfg.Create == nil ||
		cfg.List == nil ||
		cfg.GetByID == nil ||
		cfg.Update == nil ||
		cfg.Delete == nil {
		panic("registerCRUD: nil handler for base " + cfg.Base) //nolint:forbidigo // Fail fast during server setup.
	}

	wrap := func(h http.HandlerFunc) http.Handler {
		if cfg.Middleware != nil {
			return cfg.Middleware(h)
		}
		return h
	}
	mux.Handle("POST "+cfg.Base, wrap(cfg.Create))
	mux.Handle("GET "+cfg.Base, wrap(cfg.List))
	mux.Handle("GET "+cfg.Base+"/{id}", wrap(cfg.GetByID))
	mux.Handle("PUT "+cfg.Base+"/{id}", wrap(cfg.Update))
	mux.Handle("DELETE "+cfg.Base+"/{id}", wrap(cfg.Delete))
}

func registerSourceRoutes(mux *http.ServeMux, h *SourceHandlers) {
	mux.HandleFunc("POST /api/sources", h.Create)
}

func registerAuthRoutes(mux *http.ServeMux, h *AuthHandlers) {
	mux.HandleFunc("GET /auth/login", h.Login)
	mux.HandleFunc("GET /auth/callback", h.Callback)
	mux.HandleFunc("POST /auth/logout", h.Logout)
	mux.HandleFunc("GET /auth/status", h.Status)
}

// uiRouteConfig holds configuration for UI route registration.
type uiRouteConfig struct {
	Auth         *service.AuthService
	CookieDomain string
}

// authWrap returns a no-op wrapper when auth is nil, otherwise applies RequireAuthBrowser.
func (cfg uiRouteConfig) authWrap() func(http.Handler) http.Handler {
	if cfg.Auth == nil {
		return func(h http.Handler) http.Handler { return h }
	}
	return RequireAuthBrowser(cfg.Auth)
}

// adminWrap returns a no-op wrapper when auth is nil, otherwise applies RequireRoleBrowser with CSRF protection.
func (cfg uiRouteConfig) adminWrap() func(http.Handler) http.Handler {
	if cfg.Auth == nil {
		return func(h http.Handler) http.Handler { return h }
	}
	// Chain CSRF protection with admin role requirement
	csrf := CSRFProtection(CSRFConfig{CookieDomain: cfg.CookieDomain})
	roleCheck := RequireRoleBrowser(cfg.Auth, domainauth.RoleAdmin)
	return func(h http.Handler) http.Handler {
		return roleCheck(csrf(h))
	}
}

// registerUIRoutes delegates to per-domain UI route registration functions (≤3 params each).
func registerUIRoutes(mux *http.ServeMux, h *UIHandlers, cfg uiRouteConfig) {
	registerUIDashboardRoutes(mux, h, cfg)
	registerUIAlertRoutes(mux, h, cfg)
	registerUIAlertSinkRoutes(mux, h, cfg)
	registerUISecretsRoutes(mux, h, cfg)
	registerUISourcesRoutes(mux, h, cfg)
	registerUISitesRoutes(mux, h, cfg)
	registerUIJobsRoutes(mux, h, cfg)
	registerUIAllowlistRoutes(mux, h, cfg)
	registerUIIOCsRoutes(mux, h, cfg)
	// Public auth-related UI routes (no auth wrapper)
	mux.Handle("GET /auth/signed-out", http.HandlerFunc(h.SignedOut))
}

// registerUIDashboardRoutes wires main dashboard/navigation pages.
func registerUIDashboardRoutes(mux *http.ServeMux, h *UIHandlers, cfg uiRouteConfig) {
	wrap := cfg.authWrap()
	mux.Handle("GET /", wrap(http.HandlerFunc(h.Index)))
	mux.Handle("GET /dashboard", wrap(http.HandlerFunc(h.Dashboard)))
	mux.Handle("GET /dashboard/recent-browser-jobs", wrap(http.HandlerFunc(h.RecentBrowserJobsFragment)))
	mux.Handle("GET /dashboard/recent-alerts", wrap(http.HandlerFunc(h.RecentAlertsFragment)))
}

// registerUIAlertRoutes wires Alerts list and detail pages.
func registerUIAlertRoutes(mux *http.ServeMux, h *UIHandlers, cfg uiRouteConfig) {
	wrap := cfg.authWrap()
	mux.Handle("GET /alerts", wrap(http.HandlerFunc(h.Alerts)))
	mux.Handle("GET /alerts/{id}", wrap(http.HandlerFunc(h.AlertView)))
	mux.Handle("POST /alerts/{id}/resolve", wrap(http.HandlerFunc(h.AlertResolve)))
}

func registerUIAlertSinkRoutes(mux *http.ServeMux, h *UIHandlers, cfg uiRouteConfig) {
	wrap := cfg.authWrap()
	mux.Handle("GET /alert-sinks", wrap(http.HandlerFunc(h.AlertSinks)))
	mux.Handle("GET /alert-sinks/{id}", wrap(http.HandlerFunc(h.AlertSinkView)))

	wrapAdmin := cfg.adminWrap()
	mux.Handle("GET /alert-sinks/new", wrapAdmin(http.HandlerFunc(h.AlertSinkNew)))
	mux.Handle("GET /alert-sinks/{id}/edit", wrapAdmin(http.HandlerFunc(h.AlertSinkEdit)))
	mux.Handle("POST /alert-sinks", wrapAdmin(http.HandlerFunc(h.AlertSinkCreate)))
	mux.Handle("POST /alert-sinks/{id}", wrapAdmin(http.HandlerFunc(h.AlertSinkUpdate)))
	mux.Handle("POST /alert-sinks/{id}/delete", wrapAdmin(http.HandlerFunc(h.AlertSinkDelete)))
}

// registerUISecretsRoutes wires Secrets UI (admin-only) pages.
func registerUISecretsRoutes(mux *http.ServeMux, h *UIHandlers, cfg uiRouteConfig) {
	wrapAdmin := cfg.adminWrap()
	mux.Handle("GET /secrets", wrapAdmin(http.HandlerFunc(h.Secrets)))
	mux.Handle("GET /secrets/new", wrapAdmin(http.HandlerFunc(h.SecretNew)))
	mux.Handle("GET /secrets/{id}/edit", wrapAdmin(http.HandlerFunc(h.SecretEdit)))
	mux.Handle("POST /secrets", wrapAdmin(http.HandlerFunc(h.SecretCreate)))
	mux.Handle("POST /secrets/{id}", wrapAdmin(http.HandlerFunc(h.SecretUpdate)))
	mux.Handle("POST /secrets/{id}/delete", wrapAdmin(http.HandlerFunc(h.SecretDelete)))
}

func registerUISourcesRoutes(mux *http.ServeMux, h *UIHandlers, cfg uiRouteConfig) {
	wrap := cfg.authWrap()
	wrapAdmin := cfg.adminWrap()
	mux.Handle("GET /sources", wrap(http.HandlerFunc(h.Sources)))
	mux.Handle("POST /sources/{id}/delete", wrapAdmin(http.HandlerFunc(h.SourceDelete)))
	mux.Handle("GET /sources/new", wrapAdmin(http.HandlerFunc(h.SourceNew)))
	mux.Handle("GET /sources/{id}/copy", wrapAdmin(http.HandlerFunc(h.SourceCopy)))
	mux.Handle("POST /sources", wrapAdmin(http.HandlerFunc(h.SourceCreate)))

	mux.Handle("POST /sources/test", wrapAdmin(http.HandlerFunc(h.SourceTest)))
	mux.Handle("GET /sources/test/{id}/events", wrapAdmin(http.HandlerFunc(h.SourceTestEvents)))
	mux.Handle("GET /sources/test/{id}/status", wrapAdmin(http.HandlerFunc(h.SourceTestStatus)))
}

func registerUISitesRoutes(mux *http.ServeMux, h *UIHandlers, cfg uiRouteConfig) {
	wrap := cfg.authWrap()
	wrapAdmin := cfg.adminWrap()
	// List available to authenticated users
	mux.Handle("GET /sites", wrap(http.HandlerFunc(h.Sites)))
	// Admin-only create/edit flows
	mux.Handle("GET /sites/new", wrapAdmin(http.HandlerFunc(h.SiteNew)))
	mux.Handle("GET /sites/{id}/edit", wrapAdmin(http.HandlerFunc(h.SiteEdit)))
	mux.Handle("POST /sites", wrapAdmin(http.HandlerFunc(h.SiteCreate)))
	mux.Handle("POST /sites/{id}", wrapAdmin(http.HandlerFunc(h.SiteUpdate)))
	mux.Handle("POST /sites/{id}/delete", wrapAdmin(http.HandlerFunc(h.SiteDelete)))
}

func registerUIJobsRoutes(mux *http.ServeMux, h *UIHandlers, cfg uiRouteConfig) {
	wrap := cfg.authWrap()
	wrapAdmin := cfg.adminWrap()
	mux.Handle("GET /jobs", wrap(http.HandlerFunc(h.JobsList)))
	mux.Handle("GET /jobs/{id}", wrap(http.HandlerFunc(h.JobView)))
	mux.Handle("GET /jobs/{id}/events", wrap(http.HandlerFunc(h.JobEvents)))
	mux.Handle("GET /jobs/{id}/events/{eventId}", wrap(http.HandlerFunc(h.JobEventDetails)))
	mux.Handle("POST /jobs/{id}/delete", wrapAdmin(http.HandlerFunc(h.JobDelete)))
}

func registerUIAllowlistRoutes(mux *http.ServeMux, h *UIHandlers, cfg uiRouteConfig) {
	wrap := cfg.authWrap()
	wrapAdmin := cfg.adminWrap()
	// List available to authenticated users
	mux.Handle("GET /allowlist", wrap(http.HandlerFunc(h.Allowlist)))
	// Admin-only create/edit flows
	mux.Handle("GET /allowlist/new", wrapAdmin(http.HandlerFunc(h.AllowlistNew)))
	mux.Handle("GET /allowlist/{id}/edit", wrapAdmin(http.HandlerFunc(h.AllowlistEdit)))
	mux.Handle("POST /allowlist", wrapAdmin(http.HandlerFunc(h.AllowlistCreate)))
	mux.Handle("POST /allowlist/{id}", wrapAdmin(http.HandlerFunc(h.AllowlistUpdate)))
	mux.Handle("POST /allowlist/{id}/delete", wrapAdmin(http.HandlerFunc(h.AllowlistDelete)))
}

// registerUIIOCsRoutes wires IOC UI (admin-only) pages.
func registerUIIOCsRoutes(mux *http.ServeMux, h *UIHandlers, cfg uiRouteConfig) {
	wrapAdmin := cfg.adminWrap()
	mux.Handle("GET /iocs", wrapAdmin(http.HandlerFunc(h.IOCs)))
	mux.Handle("GET /iocs/new", wrapAdmin(http.HandlerFunc(h.IOCNew)))
	mux.Handle("GET /iocs/{id}/edit", wrapAdmin(http.HandlerFunc(h.IOCEdit)))
	mux.Handle("POST /iocs", wrapAdmin(http.HandlerFunc(h.IOCCreate)))
	mux.Handle("POST /iocs/{id}", wrapAdmin(http.HandlerFunc(h.IOCUpdate)))
	mux.Handle("POST /iocs/{id}/delete", wrapAdmin(http.HandlerFunc(h.IOCDelete)))
}
