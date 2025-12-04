package httpx

import (
	"bytes"
	"errors"
	"html/template"
	"io/fs"
	"log"
	"log/slog"
	"net/http"

	httpassets "github.com/target/mmk-ui-api/internal/http/assets"
	assetfuncs "github.com/target/mmk-ui-api/internal/http/templates/assets"
	corefuncs "github.com/target/mmk-ui-api/internal/http/templates/core"
	eventfuncs "github.com/target/mmk-ui-api/internal/http/templates/events"
)

// AssetResolver aliases the asset resolver so existing callers continue importing httpx.
type AssetResolver = httpassets.AssetResolver

// NewAssetResolverFromDisk creates an asset resolver that reads the manifest from the local filesystem.
func NewAssetResolverFromDisk(manifestPath string) (*AssetResolver, error) {
	return httpassets.NewAssetResolverFromDisk(manifestPath)
}

// NewAssetResolverFromFS creates an asset resolver that reads the manifest from an fs.FS implementation.
func NewAssetResolverFromFS(fsys fs.FS, manifestPath string) (*AssetResolver, error) {
	return httpassets.NewAssetResolverFromFS(fsys, manifestPath)
}

// TemplateRenderer renders HTML templates for UI responses.
type TemplateRenderer struct {
	t             *template.Template
	resolver      *AssetResolver
	criticalCSSFS fs.FS        // For hot reloading in dev mode
	criticalCSS   string       // Cached for production mode
	devMode       bool         // Whether to reload CSS on each request
	logger        *slog.Logger // For logging template errors
}

// TemplateRendererConfig holds configuration for creating a TemplateRenderer.
type TemplateRendererConfig struct {
	TemplateFS    fs.FS          // Filesystem containing templates (required)
	Resolver      *AssetResolver // Asset resolver for hashed filenames (optional)
	CriticalCSSFS fs.FS          // Filesystem containing css/critical.css (optional)
	DevMode       bool           // Enable hot reloading of critical CSS
	Logger        *slog.Logger   // Logger for template errors (optional)
}

// NewTemplateRenderer constructs a renderer by parsing templates from the provided config.
// In dev mode, criticalCSSFS should be os.DirFS("frontend/public") to read "css/critical.css".
// In prod mode, criticalCSSFS should be fs.Sub(StaticFS, "frontend/static") to read "css/critical.css".
// Set DevMode=true to enable hot reloading of critical CSS on each request (dev only).
func NewTemplateRenderer(cfg TemplateRendererConfig) (*TemplateRenderer, error) {
	if cfg.TemplateFS == nil {
		return nil, errors.New("TemplateFS is required")
	}

	// Load critical CSS once at startup (for production) or prepare for hot reloading (dev)
	var criticalCSS string
	if cfg.CriticalCSSFS != nil && !cfg.DevMode {
		// Production: load once and cache
		cssBytes, err := fs.ReadFile(cfg.CriticalCSSFS, "css/critical.css")
		if err != nil {
			log.Printf("Warning: failed to load critical CSS from css/critical.css: %v", err)
			// Fallback: use minimal critical CSS
			criticalCSS = ":root{--color-background:#f6f7f9;--color-surface:#fff;--color-text-primary:#2e3138;}"
		} else {
			criticalCSS = string(cssBytes)
		}
	}

	renderer := &TemplateRenderer{
		resolver:      cfg.Resolver,
		criticalCSSFS: cfg.CriticalCSSFS,
		criticalCSS:   criticalCSS,
		devMode:       cfg.DevMode,
		logger:        cfg.Logger,
	}

	var t *template.Template
	funcs := createTemplateFuncs(&t, renderer)
	var err error
	t, err = template.New("root").Funcs(funcs).ParseFS(cfg.TemplateFS,
		"*.tmpl",
		"pages/*.tmpl",
		"partials/*.tmpl",
		"partials/events/*.tmpl",
	)
	if err != nil {
		if cfg.Logger != nil {
			cfg.Logger.Error("template parsing failed",
				slog.Any("error", err),
				slog.String("phase", "initialization"),
			)
		}
		return nil, err
	}
	renderer.t = t
	return renderer, nil
}

// getCriticalCSS returns the critical CSS, reloading from disk in dev mode.
func (r *TemplateRenderer) getCriticalCSS() string {
	if r.devMode && r.criticalCSSFS != nil {
		// Dev mode: reload from disk on each request for hot reloading
		cssBytes, err := fs.ReadFile(r.criticalCSSFS, "css/critical.css")
		if err != nil {
			log.Printf("Warning: failed to reload critical CSS in dev mode: %v", err)
			return ":root{--color-background:#f6f7f9;--color-surface:#fff;--color-text-primary:#2e3138;}"
		}
		return string(cssBytes)
	}
	// Production mode: return cached CSS
	return r.criticalCSS
}

// RenderFull renders the full page (layout + page content). Keep ≤3 params.
func (r *TemplateRenderer) RenderFull(w http.ResponseWriter, _ *http.Request, data any) error {
	return r.renderTemplate(w, "layout", data)
}

// RenderPartial renders only the main content area.
func (r *TemplateRenderer) RenderPartial(w http.ResponseWriter, _ *http.Request, data any) error {
	return r.renderTemplate(w, "content", data)
}

// RenderError renders an error page using the error template.
func (r *TemplateRenderer) RenderError(w http.ResponseWriter, _ *http.Request, data any) error {
	// Use the error.tmpl template which defines "error-layout"
	return r.renderTemplate(w, "error-layout", data)
}

func (r *TemplateRenderer) renderTemplate(w http.ResponseWriter, templateName string, data any) error {
	var buf bytes.Buffer
	if err := r.t.ExecuteTemplate(&buf, templateName, data); err != nil {
		r.logTemplateError(templateName, err)
		return err
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if _, err := buf.WriteTo(w); err != nil {
		if r.logger != nil {
			r.logger.Error("failed to write rendered template",
				slog.String("template", templateName),
				slog.Any("error", err),
			)
		}
		return err
	}

	return nil
}

// logTemplateError logs a template execution error with context.
func (r *TemplateRenderer) logTemplateError(templateName string, err error) {
	if r.logger == nil || err == nil {
		return
	}
	r.logger.Error("template execution failed",
		slog.String("template", templateName),
		slog.Any("error", err),
	)
}

func createTemplateFuncs(t **template.Template, renderer *TemplateRenderer) template.FuncMap {
	funcs := template.FuncMap{}

	mergeTemplateFuncs(funcs,
		corefuncs.Funcs(corefuncs.Deps{
			Template:           t,
			ContentTemplateFor: ContentTemplateFor,
		}),
		assetfuncs.Funcs(assetfuncs.Options{
			Resolver:    renderer.resolver,
			DevMode:     renderer.devMode,
			CriticalCSS: renderer.getCriticalCSS,
		}),
		eventfuncs.Funcs(eventfuncs.Deps{
			Template: t,
		}),
	)

	return funcs
}

func mergeTemplateFuncs(dst template.FuncMap, sources ...template.FuncMap) {
	for _, src := range sources {
		for key, val := range src {
			dst[key] = val
		}
	}
}
