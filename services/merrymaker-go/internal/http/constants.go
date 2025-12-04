package httpx

// CurrentPage constants define the page identifiers used in templates and navigation.
// These constants ensure consistency across UI handlers and template mapping.
const (
	// Main navigation pages.
	PageHome      = "home"
	PageDashboard = "dashboard"

	// Alert-related pages.
	PageAlerts        = "alerts"
	PageAlertView     = "alert-view" // alert detail view
	PageAlertSinks    = "alert-sinks"
	PageAlertSink     = "alert-sink"      // view page
	PageAlertSinkForm = "alert-sink-form" // create/edit form

	// Backward compatibility - maps to alert sink view.
	PageAlert = "alert"

	// Secret-related pages.
	PageSecrets    = "secrets"
	PageSecretForm = "secret-form"

	// Source-related pages.
	PageSources    = "sources"
	PageSourceForm = "source-form"

	// Site-related pages.
	PageSites    = "sites"
	PageSiteForm = "site-form"

	// Domain Allowlist pages.
	PageAllowlist     = "allowlist"
	PageAllowlistForm = "allowlist-form"

	// IOC pages.
	PageIOCs    = "iocs"
	PageIOCForm = "ioc-form"

	// Job-related pages.
	PageJobs = "jobs" // Admin jobs list
	PageJob  = "job"
)

const (
	// DefaultScope is the default scope for rules processing when a site has no explicit scope.
	DefaultScope = "default"

	// MaxSitesForFilter is the maximum number of sites to fetch for filter dropdowns.
	// This prevents excessive memory usage and query time for large deployments.
	MaxSitesForFilter = 1000
)

// Template paths used for loading templates in tests and production.
const (
	// Template directory paths.
	TemplatePathFromRoot = "frontend/templates"       // From project root
	TemplatePathFromTest = "../../frontend/templates" // From internal/http test files
)

// FormMode represents the mode of a form (create or edit).
// Using a dedicated type improves compile-time checks and prevents typos.
type FormMode string

const (
	// FormModeEdit indicates the form is in edit mode.
	FormModeEdit FormMode = "edit"
	// FormModeCreate indicates the form is in create mode.
	FormModeCreate FormMode = "create"
)

// Content templates are defined once and reused to avoid per-call allocations.
//
//nolint:gochecknoglobals // static read-only lookup for templates; avoids per-call allocations
var contentTemplates = map[string]string{
	PageHome:          "dashboard-content", // Home page now shows dashboard
	PageDashboard:     "dashboard-content",
	PageAlerts:        "alerts-content",
	PageAlertView:     "alert-view-content",
	PageAlert:         "alert-sink-view-content", // backward-compat key
	PageAlertSinks:    "alert-sinks-content",
	PageSecrets:       "secrets-content",
	PageSecretForm:    "secret-form-content",
	PageAlertSink:     "alert-sink-view-content",
	PageAlertSinkForm: "alert-sink-form-content",
	PageSources:       "sources-content",
	PageSourceForm:    "source-form-content",
	PageSites:         "sites-content",
	PageSiteForm:      "site-form-content",
	PageAllowlist:     "allowlist-content",
	PageAllowlistForm: "allowlist-form-content",
	PageIOCs:          "iocs-content",
	PageIOCForm:       "ioc-form-content",
	PageJobs:          "jobs-content",
	PageJob:           "job-content",
}

// ContentTemplateMap returns the mapping from CurrentPage to template name.
// This is the single source of truth for page-to-template mapping.
func ContentTemplateMap() map[string]string { return contentTemplates }

// ContentTemplateFor returns the content template for the given CurrentPage.
// Falls back to dashboard-content for unknown pages.
func ContentTemplateFor(currentPage string) string {
	if name, ok := ContentTemplateMap()[currentPage]; ok {
		return name
	}
	return "dashboard-content"
}
