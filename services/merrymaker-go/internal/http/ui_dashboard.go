package httpx

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	alertsvm "github.com/target/mmk-ui-api/internal/http/ui/alerts"
	"github.com/target/mmk-ui-api/internal/http/ui/viewmodel"
	"github.com/target/mmk-ui-api/internal/http/uiutil"
)

const errMsgUnableLoadRecentJobs = "Unable to load recent browser jobs"

// DashboardJob represents a job with site name for dashboard display.
type DashboardJob struct {
	ID          string
	SiteName    string
	Status      model.JobStatus
	CreatedAt   time.Time
	CompletedAt *time.Time
	LastError   *string
}

// FriendlyCreatedAt returns a human friendly description of when the job started.
func (j DashboardJob) FriendlyCreatedAt() string {
	return uiutil.FriendlyRelativeTime(j.CreatedAt)
}

// FriendlyCompletedAt returns a human friendly description of when the job completed.
func (j DashboardJob) FriendlyCompletedAt() string {
	if j.CompletedAt == nil {
		return ""
	}
	return uiutil.FriendlyRelativeTime(*j.CompletedAt)
}

// FailureSummary returns a short version of the last error for failed jobs.
func (j DashboardJob) FailureSummary() string {
	if j.LastError == nil {
		return ""
	}
	text := strings.TrimSpace(*j.LastError)
	if text == "" {
		return ""
	}
	return uiutil.TruncateWithEllipsis(text, 90)
}

// StatusSummary returns a concise string describing job status and timing.
func (j DashboardJob) StatusSummary() string {
	switch j.Status {
	case model.JobStatusFailed:
		if completed := j.FriendlyCompletedAt(); completed != "" {
			return "Failed " + completed
		}
		return "Failed " + j.FriendlyCreatedAt()
	case model.JobStatusCompleted:
		if completed := j.FriendlyCompletedAt(); completed != "" {
			return "Completed " + completed
		}
		return "Completed"
	case model.JobStatusRunning:
		return "Running " + j.FriendlyCreatedAt()
	case model.JobStatusPending:
		return "Queued " + j.FriendlyCreatedAt()
	default:
		return j.FriendlyCreatedAt()
	}
}

// Index serves the home page with dashboard content.
func (h *UIHandlers) Index(w http.ResponseWriter, r *http.Request) {
	h.Page(w, r, PageSpec{
		Meta: PageMeta{Title: "Merrymaker - Dashboard", PageTitle: "Dashboard", CurrentPage: PageHome},
		Fetch: func(ctx context.Context, data map[string]any) error {
			h.populateRecentJobs(ctx, data)
			h.populateRecentAlerts(ctx, data)
			return nil
		},
	})
}

// fetchRecentJobsWithSiteNames fetches recent browser jobs with site names, excluding test jobs.
// This method uses a JOIN-based query to eliminate N+1 lookups when fetching site names.
// Test jobs are filtered at the query layer for efficiency.
func (h *UIHandlers) fetchRecentJobsWithSiteNames(ctx context.Context, limit int) []DashboardJob {
	jobs, err := h.Jobs.ListRecentByTypeWithSiteNames(ctx, model.JobTypeBrowser, limit)
	if err != nil {
		h.logger().WarnContext(ctx, "failed to fetch recent jobs for dashboard", "error", err)
		return []DashboardJob{}
	}

	result := make([]DashboardJob, 0, len(jobs))
	for _, job := range jobs {
		// Only include jobs with a site_id
		if job.SiteID == nil || *job.SiteID == "" {
			continue
		}

		result = append(result, DashboardJob{
			ID:          job.ID,
			SiteName:    job.SiteName,
			Status:      job.Status,
			CreatedAt:   job.CreatedAt,
			CompletedAt: job.CompletedAt,
			LastError:   job.LastError,
		})
	}

	return result
}

func (h *UIHandlers) populateRecentJobs(ctx context.Context, data map[string]any) {
	data["RecentBrowserJobs"] = []DashboardJob{}
	data["RecentBrowserJobsError"] = ""

	if h.Jobs == nil || h.SiteSvc == nil {
		data["RecentBrowserJobsError"] = errMsgUnableLoadRecentJobs
		return
	}

	data["RecentBrowserJobs"] = h.fetchRecentJobsWithSiteNames(ctx, 5)
}

func (h *UIHandlers) populateRecentAlerts(ctx context.Context, data map[string]any) {
	data["RecentAlerts"] = []alertsvm.AlertRow{}
	data["RecentAlertsError"] = ""

	if h.AlertsSvc == nil {
		data["RecentAlertsError"] = errMsgUnableLoadAlerts
		return
	}

	alerts, err := h.fetchRecentAlerts(ctx, 5)
	if err != nil {
		data["RecentAlertsError"] = errMsgUnableLoadAlerts
		return
	}

	data["RecentAlerts"] = alerts
}

// fetchRecentAlerts fetches recent alerts with site names for dashboard display.
func (h *UIHandlers) fetchRecentAlerts(ctx context.Context, limit int) ([]alertsvm.AlertRow, error) {
	if h.AlertsSvc == nil {
		return nil, errors.New("alerts service is not available")
	}

	opts := &model.AlertListOptions{
		Limit:  limit,
		Offset: 0,
		Sort:   "fired_at",
		Dir:    SortDirDesc,
	}

	alerts, err := h.AlertsSvc.ListWithSiteNames(ctx, opts)
	if err != nil {
		h.logger().WarnContext(ctx, "failed to fetch recent alerts for dashboard", "error", err)
		return nil, fmt.Errorf("fetch recent alerts: %w", err)
	}

	rows := make([]alertsvm.AlertRow, 0, len(alerts))
	for _, alert := range alerts {
		rows = append(rows, h.toAlertRowFromAlertWithSiteName(alert))
	}
	return rows, nil
}

// RecentBrowserJobsFragment serves the recent browser jobs panel for HTMX polling.
func (h *UIHandlers) RecentBrowserJobsFragment(w http.ResponseWriter, r *http.Request) {
	data := basePageData(r, PageMeta{})
	data["RecentBrowserJobs"] = []DashboardJob{}
	data["RecentBrowserJobsError"] = ""
	if h.Jobs != nil && h.SiteSvc != nil {
		data["RecentBrowserJobs"] = h.fetchRecentJobsWithSiteNames(r.Context(), 5)
	} else {
		data["RecentBrowserJobsError"] = errMsgUnableLoadRecentJobs
	}

	h.renderFragment(w, r, fragmentRenderOptions{
		Template: "dashboard-recent-browser-jobs-fragment",
		Data:     data,
	})
}

// RecentAlertsFragment serves the alerts panel with periodic refresh support.
func (h *UIHandlers) RecentAlertsFragment(w http.ResponseWriter, r *http.Request) {
	data := basePageData(r, PageMeta{})
	data["RecentAlerts"] = []alertsvm.AlertRow{}
	data["RecentAlertsError"] = ""
	if alerts, err := h.fetchRecentAlerts(r.Context(), 5); err == nil {
		data["RecentAlerts"] = alerts
	} else {
		data["RecentAlertsError"] = errMsgUnableLoadAlerts
	}

	h.renderFragment(w, r, fragmentRenderOptions{
		Template: "dashboard-recent-alerts-fragment",
		Data:     data,
	})
}

// renderFragment renders an HTMX fragment with consistent headers and logging.
type fragmentRenderOptions struct {
	Template string
	Data     map[string]any
}

func (h *UIHandlers) renderFragment(w http.ResponseWriter, r *http.Request, opts fragmentRenderOptions) {
	var buf bytes.Buffer
	if err := h.T.t.ExecuteTemplate(&buf, opts.Template, opts.Data); err != nil {
		h.logger().ErrorContext(r.Context(), "failed to render fragment",
			"template", opts.Template,
			"error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store, no-cache, must-revalidate, max-age=0")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Vary", "HX-Request")
	if _, err := buf.WriteTo(w); err != nil {
		h.logger().ErrorContext(r.Context(), "failed to write fragment",
			"template", opts.Template,
			"error", err)
	}
}

// Dashboard redirects to the home page (dashboard is now at "/").
func (h *UIHandlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/", http.StatusMovedPermanently)
}

// Alerts serves the fired alerts page with pagination and filtering.
func (h *UIHandlers) Alerts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	pageNum, pageSize := getPageParams(query)
	filters := parseAlertsFilter(query)

	page := h.newAlertsPage(r, filters, pageNum, pageSize)

	if h.AlertsSvc == nil {
		page.Error = true
		page.ErrorMessage = errMsgUnableLoadAlerts
		h.renderDashboardPage(w, r, page)
		return
	}

	rows, err := h.fetchAlertsWithFilters(r.Context(), filters, pageOpts{Page: pageNum, PageSize: pageSize})
	if err != nil {
		h.logger().Error("failed to load alerts for UI", "error", err)
		page.Error = true
		page.ErrorMessage = errMsgUnableLoadAlerts
		h.renderDashboardPage(w, r, page)
		return
	}

	h.hydrateAlertsPage(page, rows, alertsPageHydration{
		PageNum:  pageNum,
		PageSize: pageSize,
		Query:    query,
	})

	h.renderDashboardPage(w, r, page)
}

func (h *UIHandlers) newAlertsPage(r *http.Request, filters alertsFilter, pageNum, pageSize int) *alertsvm.Page {
	layout := buildLayout(r, PageMeta{Title: "Merrymaker - Alerts", PageTitle: "Alerts", CurrentPage: PageAlerts})

	page := &alertsvm.Page{
		Layout: layout,
		Pagination: viewmodel.Pagination{
			Page:     pageNum,
			PageSize: pageSize,
		},
		SiteID:     filters.SiteID,
		Severity:   filters.Severity,
		RuleType:   filters.RuleType,
		Unresolved: filters.Unresolved,
		Sort:       filters.Sort,
		Dir:        filters.Dir,
		SeverityOptions: []string{
			string(model.AlertSeverityCritical),
			string(model.AlertSeverityHigh),
			string(model.AlertSeverityMedium),
			string(model.AlertSeverityLow),
			string(model.AlertSeverityInfo),
		},
		RuleTypeOptions: []string{
			string(model.AlertRuleTypeUnknownDomain),
			string(model.AlertRuleTypeIOC),
			string(model.AlertRuleTypeYaraRule),
			string(model.AlertRuleTypeCustom),
		},
	}

	page.SiteOptions = h.fetchSitesForFilter(r.Context())
	return page
}

type alertsPageHydration struct {
	PageNum  int
	PageSize int
	Query    url.Values
}

func (h *UIHandlers) hydrateAlertsPage(
	page *alertsvm.Page,
	rows []alertsvm.AlertRow,
	opts alertsPageHydration,
) {
	hasNext := len(rows) > opts.PageSize
	if hasNext {
		rows = rows[:opts.PageSize]
	}
	page.Alerts = rows

	if len(rows) > 0 {
		offset := (opts.PageNum - 1) * opts.PageSize
		page.StartIndex = offset + 1
		page.EndIndex = offset + len(rows)
	} else {
		page.StartIndex = 0
		page.EndIndex = 0
	}

	page.HasPrev = opts.PageNum > 1
	page.HasNext = hasNext
	if page.HasPrev {
		page.PrevURL = buildPageURL("/alerts", opts.Query, pageOpts{Page: opts.PageNum - 1, PageSize: opts.PageSize})
	}
	if page.HasNext {
		page.NextURL = buildPageURL("/alerts", opts.Query, pageOpts{Page: opts.PageNum + 1, PageSize: opts.PageSize})
	}
}
