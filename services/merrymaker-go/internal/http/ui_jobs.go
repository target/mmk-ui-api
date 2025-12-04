package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/util"
	"golang.org/x/sync/errgroup"
)

const sourcePreviewLimit = 200

const (
	defaultEventsPageSize = 50 // Default page size for job events pagination
	defaultEventSortBy    = "created_at"
	defaultEventSortDir   = "asc"
)

// truncateSourcePreview truncates a source value to sourcePreviewLimit runes to avoid breaking UTF-8.
func truncateSourcePreview(value string) string {
	runes := []rune(value)
	if len(runes) > sourcePreviewLimit {
		return string(runes[:sourcePreviewLimit]) + "..."
	}
	return value
}

// jobEventFilters holds the parsed filter parameters for job events.
type jobEventFilters struct {
	EventType   *string
	Category    *string
	SearchQuery *string
	SortBy      *string
	SortDir     *string
}

// parseJobEventFilters parses filter parameters from the request.
// Supports combined sort parameter (e.g., "timestamp:asc") or separate sort/dir params.
func parseJobEventFilters(r *http.Request) jobEventFilters {
	filters := jobEventFilters{}

	if eventType := strings.TrimSpace(r.URL.Query().Get("event_type")); eventType != "" {
		filters.EventType = &eventType
	}

	if category := strings.TrimSpace(r.URL.Query().Get("category")); category != "" {
		filters.Category = &category
	}

	if searchQuery := strings.TrimSpace(r.URL.Query().Get("q")); searchQuery != "" {
		filters.SearchQuery = &searchQuery
	}

	sortBy, sortDir := ParseSortParam(r.URL.Query(), "sort", "dir")

	if sortBy != "" {
		filters.SortBy = &sortBy
	}

	if sortDir != "" {
		filters.SortDir = &sortDir
	}

	return filters
}

// fetchSiteForJob fetches site information for a job.
func (h *UIHandlers) fetchSiteForJob(ctx context.Context, job *model.Job) *model.Site {
	if job.SiteID == nil || *job.SiteID == "" || h.SiteSvc == nil {
		return nil
	}
	site, err := h.SiteSvc.GetByID(ctx, *job.SiteID)
	if err != nil {
		log.Printf("fetchSiteForJob: failed to fetch site %s: %v", *job.SiteID, err)
		return nil
	}
	if site == nil {
		log.Printf("fetchSiteForJob: site %s not found", *job.SiteID)
	}
	return site
}

// fetchSourceForJob fetches source information for a job.
func (h *UIHandlers) fetchSourceForJob(ctx context.Context, job *model.Job) *model.Source {
	if job.SourceID == nil || *job.SourceID == "" || h.SourceSvc == nil {
		return nil
	}
	source, err := h.SourceSvc.GetByID(ctx, *job.SourceID)
	if err != nil {
		log.Printf("fetchSourceForJob: failed to fetch source %s: %v", *job.SourceID, err)
		return nil
	}
	if source == nil {
		log.Printf("fetchSourceForJob: source %s not found", *job.SourceID)
	}
	return source
}

// SecretView is a typed, safe view model for rendering Secret context in job UI.
// Only includes non-sensitive fields needed by templates.
type SecretView struct {
	ID                string
	Name              string
	LastRefreshedAt   *time.Time
	LastRefreshStatus *string
	LastRefreshError  *string
}

type rulesSummaryCard struct {
	Title     string
	Icon      string
	Value     int
	ValueText string
	Context   string
}

type rulesMetricRow struct {
	Label       string
	Count       int
	Samples     []string
	BadgeClass  string
	Description string
}

type rulesResultsView struct {
	Summary        []rulesSummaryCard
	UnknownMetrics []rulesMetricRow
	IOCMetrics     []rulesMetricRow
	ProcessingTime string
	EventsSkipped  int
	AlertMode      model.SiteAlertMode
	AlertModeStr   string
	MutedAlerts    int
	IsMutedMode    bool
}

type siteView struct {
	*model.Site
	AlertModeStr string
}

type alertDeliveryView struct {
	SinkID         string
	SinkName       string
	Status         string
	AttemptNumber  int
	RetryCount     int
	MaxRetries     int
	RetriesAllowed int
	AttemptedAt    time.Time
	CompletedAt    *time.Time
	Duration       string
	ErrorMessage   string
	Payload        string
	Request        alertDeliveryRequestView
	Response       *alertDeliveryResponseView
}

type alertDeliveryRequestView struct {
	Method        string
	URL           string
	Headers       []headerKV
	Body          string
	BodyTruncated bool
	OkStatus      int
}

type alertDeliveryResponseView struct {
	StatusCode    int
	Headers       []headerKV
	Body          string
	BodyTruncated bool
}

type headerKV struct {
	Key   string
	Value string
}

func calcRulesAlertCounts(res *service.RulesProcessingResults, alertMode model.SiteAlertMode) (int, int) {
	if res == nil {
		return 0, 0
	}

	muted := res.UnknownDomain.AlertedMuted.Count + res.IOC.AlertsMuted.Count
	if alertMode == model.SiteAlertModeMuted && muted < res.AlertsCreated {
		muted = res.AlertsCreated
	}

	delivered := res.AlertsCreated - muted
	if delivered < 0 {
		delivered = 0
	}
	return delivered, muted
}

func buildRulesSummaryCards(res *service.RulesProcessingResults, alertMode model.SiteAlertMode) []rulesSummaryCard {
	deliveredAlerts, mutedAlerts := calcRulesAlertCounts(res, alertMode)
	modeLabel := "Active"
	if alertMode == model.SiteAlertModeMuted {
		modeLabel = "Muted"
	}

	return []rulesSummaryCard{
		{Title: "Alerts Delivered", Icon: "shield-check", Value: deliveredAlerts, Context: "Sent to sinks"},
		{Title: "Alerts Muted", Icon: "bell-off", Value: mutedAlerts, Context: "Site alert mode: " + modeLabel},
		{
			Title:   "Domains Processed",
			Icon:    "globe-2",
			Value:   res.DomainsProcessed,
			Context: "Network events evaluated",
		},
		{
			Title:   "IOC Matches",
			Icon:    "target",
			Value:   res.IOC.Matches.Count,
			Context: "Indicators of compromise observed",
		},
	}
}

func buildUnknownMetricRows(res *service.RulesProcessingResults, alertMode model.SiteAlertMode) []rulesMetricRow {
	rows := []rulesMetricRow{
		{
			Label:       "Alerts Delivered",
			Count:       res.UnknownDomain.Alerted.Count,
			Samples:     res.UnknownDomain.Alerted.Samples,
			BadgeClass:  "badge badge-success",
			Description: "Alerts dispatched for first-seen domains",
		},
	}

	if alertMode == model.SiteAlertModeMuted || res.UnknownDomain.AlertedMuted.Count > 0 {
		rows = append(rows, rulesMetricRow{
			Label:       "Alerts Muted",
			Count:       res.UnknownDomain.AlertedMuted.Count,
			Samples:     res.UnknownDomain.AlertedMuted.Samples,
			BadgeClass:  "badge badge-warning",
			Description: "Suppressed because the site alert mode is muted",
		})
	}

	rows = append(rows,
		rulesMetricRow{
			Label:       "Allowlisted",
			Count:       res.UnknownDomain.SuppressedAllowlist.Count,
			Samples:     res.UnknownDomain.SuppressedAllowlist.Samples,
			BadgeClass:  "badge badge-secondary",
			Description: "Suppressed by the domain allowlist",
		},
		rulesMetricRow{
			Label:       "Already Seen",
			Count:       res.UnknownDomain.SuppressedSeen.Count,
			Samples:     res.UnknownDomain.SuppressedSeen.Samples,
			BadgeClass:  "badge badge-secondary",
			Description: "Previously observed within this scope",
		},
		rulesMetricRow{
			Label:       "Alert Once (TTL)",
			Count:       res.UnknownDomain.SuppressedDedupe.Count,
			Samples:     res.UnknownDomain.SuppressedDedupe.Samples,
			BadgeClass:  "badge badge-secondary",
			Description: "Suppressed by alert-once dedupe window",
		},
		rulesMetricRow{
			Label:       "Normalization Issues",
			Count:       res.UnknownDomain.NormalizationFailed.Count,
			Samples:     res.UnknownDomain.NormalizationFailed.Samples,
			BadgeClass:  "badge badge-warning",
			Description: "Dropped due to invalid domain parsing",
		},
		rulesMetricRow{
			Label:       "Evaluation Errors",
			Count:       res.UnknownDomain.Errors.Count,
			Samples:     res.UnknownDomain.Errors.Samples,
			BadgeClass:  "badge badge-danger",
			Description: "Rule evaluation errors encountered",
		},
	)

	return rows
}

func buildIOCMetricsRows(res *service.RulesProcessingResults, alertMode model.SiteAlertMode) []rulesMetricRow {
	var rows []rulesMetricRow

	if res.IOC.Matches.Count > 0 {
		rows = append(rows, rulesMetricRow{
			Label:       "Matches Observed",
			Count:       res.IOC.Matches.Count,
			Samples:     res.IOC.Matches.Samples,
			BadgeClass:  "badge badge-info",
			Description: "Indicators of compromise detected during the run",
		})
	}

	if res.IOC.Alerts.Count > 0 {
		rows = append(rows, rulesMetricRow{
			Label:       "Alerts Delivered",
			Count:       res.IOC.Alerts.Count,
			Samples:     res.IOC.Alerts.Samples,
			BadgeClass:  "badge badge-success",
			Description: "Alerts dispatched for IOC matches",
		})
	}

	if alertMode == model.SiteAlertModeMuted || res.IOC.AlertsMuted.Count > 0 {
		rows = append(rows, rulesMetricRow{
			Label:       "Alerts Muted",
			Count:       res.IOC.AlertsMuted.Count,
			Samples:     res.IOC.AlertsMuted.Samples,
			BadgeClass:  "badge badge-warning",
			Description: "Suppressed because the site alert mode is muted",
		})
	}

	return rows
}

// fetchSecretForJob fetches secret information for a secret_refresh job.
// Returns a typed view model with safe fields (no secret value exposed).
func (h *UIHandlers) fetchSecretForJob(ctx context.Context, job *model.Job) *SecretView {
	if job.Type != model.JobTypeSecretRefresh || h.SecretSvc == nil {
		return nil
	}

	// Parse job payload to get secret_id
	var payload struct {
		SecretID string `json:"secret_id"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		log.Printf("fetchSecretForJob: failed to parse payload: %v", err)
		return nil
	}
	if payload.SecretID == "" {
		return nil
	}

	secret, err := h.SecretSvc.GetByID(ctx, payload.SecretID)
	if err != nil {
		log.Printf("fetchSecretForJob: failed to fetch secret %s: %v", payload.SecretID, err)
		return nil
	}
	if secret == nil {
		log.Printf("fetchSecretForJob: secret %s not found", payload.SecretID)
		return nil
	}

	// Return safe fields only (no secret value)
	return &SecretView{
		ID:                secret.ID,
		Name:              secret.Name,
		LastRefreshedAt:   secret.LastRefreshedAt,
		LastRefreshStatus: secret.LastRefreshStatus,
		LastRefreshError:  secret.LastRefreshError,
	}
}

// enrichJobContext fetches site, source, and secret information concurrently and adds them to template data.
func (h *UIHandlers) enrichJobContext(ctx context.Context, job *model.Job, data map[string]any) {
	g, gctx := errgroup.WithContext(ctx)
	var site *model.Site
	var source *model.Source
	var secret *SecretView

	// Fetch site concurrently
	g.Go(func() error {
		site = h.fetchSiteForJob(gctx, job)
		return nil
	})

	// Fetch source concurrently
	g.Go(func() error {
		source = h.fetchSourceForJob(gctx, job)
		return nil
	})

	// Fetch secret concurrently (for secret_refresh jobs)
	g.Go(func() error {
		secret = h.fetchSecretForJob(gctx, job)
		return nil
	})

	// Wait for all fetches to complete
	if err := g.Wait(); err != nil {
		log.Printf("enrichJobContext: background fetch failed: %v", err)
	}

	// Add site data if fetched successfully
	populateSiteData(data, site)

	// Add source data if fetched successfully
	if source != nil {
		data["Source"] = source
		data["SourcePreview"] = truncateSourcePreview(source.Value)
	}

	// Add secret data if fetched successfully (for secret_refresh jobs)
	if secret != nil {
		data["Secret"] = secret
	}
}

func buildRulesResultsView(res *service.RulesProcessingResults, alertMode model.SiteAlertMode) *rulesResultsView {
	if res == nil {
		return nil
	}

	if normalized, ok := model.ParseSiteAlertMode(string(res.AlertMode)); ok {
		res.AlertMode = normalized
	}
	if normalized, ok := model.ParseSiteAlertMode(string(alertMode)); ok {
		alertMode = normalized
	}

	if !alertMode.Valid() {
		alertMode = res.AlertMode
	}
	if !alertMode.Valid() {
		alertMode = model.SiteAlertModeActive
	}

	_, mutedAlerts := calcRulesAlertCounts(res, alertMode)

	return &rulesResultsView{
		Summary:        buildRulesSummaryCards(res, alertMode),
		UnknownMetrics: buildUnknownMetricRows(res, alertMode),
		IOCMetrics:     buildIOCMetricsRows(res, alertMode),
		ProcessingTime: util.FormatProcessingDuration(res.ProcessingTime),
		EventsSkipped:  res.EventsSkipped,
		AlertMode:      alertMode,
		AlertModeStr:   string(alertMode),
		MutedAlerts:    mutedAlerts,
		IsMutedMode:    alertMode == model.SiteAlertModeMuted,
	}
}

func populateSiteData(data map[string]any, site *model.Site) {
	if site == nil {
		return
	}

	if normalized, ok := model.ParseSiteAlertMode(string(site.AlertMode)); ok {
		site.AlertMode = normalized
	}

	data["Site"] = &siteView{
		Site:         site,
		AlertModeStr: string(site.AlertMode),
	}
	scope := extractSiteScope(site)
	if scope != "" {
		data["SiteScope"] = scope
	}
}

func extractSiteScope(site *model.Site) string {
	if site.Scope == nil {
		return ""
	}

	return strings.TrimSpace(*site.Scope)
}

func buildAlertDeliveryView(res *service.AlertDeliveryJobResult) *alertDeliveryView {
	if res == nil {
		return nil
	}

	attemptNumber := res.AttemptNumber
	if attemptNumber <= 0 {
		attemptNumber = res.RetryCount + 1
	}

	attemptedAt := res.AttemptedAt
	if attemptedAt.IsZero() && res.CompletedAt != nil {
		attemptedAt = *res.CompletedAt
	}

	duration := ""
	if res.DurationMs > 0 {
		duration = util.FormatProcessingDuration(time.Duration(res.DurationMs) * time.Millisecond)
	}

	view := &alertDeliveryView{
		SinkID:         res.SinkID,
		SinkName:       res.SinkName,
		Status:         string(res.JobStatus),
		AttemptNumber:  attemptNumber,
		RetryCount:     res.RetryCount,
		MaxRetries:     res.MaxRetries,
		RetriesAllowed: calcRetriesAllowed(res.MaxRetries),
		AttemptedAt:    attemptedAt,
		CompletedAt:    res.CompletedAt,
		Duration:       duration,
		ErrorMessage:   res.ErrorMessage,
		Payload:        prettyJSON(res.Payload),
		Request: alertDeliveryRequestView{
			Method:        res.Request.Method,
			URL:           res.Request.URL,
			Headers:       mapToHeaderList(res.Request.Headers),
			Body:          res.Request.Body,
			BodyTruncated: res.Request.BodyTruncated,
			OkStatus:      res.Request.OkStatus,
		},
	}

	if res.Response != nil {
		view.Response = &alertDeliveryResponseView{
			StatusCode:    res.Response.StatusCode,
			Headers:       mapToHeaderList(res.Response.Headers),
			Body:          res.Response.Body,
			BodyTruncated: res.Response.BodyTruncated,
		}
	}

	return view
}

func calcRetriesAllowed(maxRetries int) int {
	if maxRetries <= 1 {
		return 0
	}
	return maxRetries - 1
}

func mapToHeaderList(headers map[string]string) []headerKV {
	if len(headers) == 0 {
		return nil
	}
	type headerEntry struct {
		key   string
		lower string
	}
	entries := make([]headerEntry, 0, len(headers))
	for k := range headers {
		entries = append(entries, headerEntry{
			key:   k,
			lower: strings.ToLower(k),
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].lower < entries[j].lower
	})
	out := make([]headerKV, 0, len(entries))
	for _, entry := range entries {
		out = append(out, headerKV{Key: entry.key, Value: headers[entry.key]})
	}
	return out
}

func prettyJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var buf bytes.Buffer
	if err := json.Indent(&buf, raw, "", "  "); err != nil {
		return string(raw)
	}
	return buf.String()
}

func (h *UIHandlers) loadJobForView(r *http.Request, jobID string, data map[string]any) *model.Job {
	if h.Jobs == nil || jobID == "" {
		return nil
	}
	job, err := h.Jobs.GetByID(r.Context(), jobID)
	if err != nil || job == nil {
		return nil
	}

	data["Job"] = job
	data["CreatedAtFormatted"] = job.CreatedAt.UTC().Format("2006-01-02 15:04:05 MST")
	if job.CompletedAt != nil {
		data["CompletedAtFormatted"] = job.CompletedAt.UTC().Format("2006-01-02 15:04:05 MST")
	}

	h.enrichJobContext(r.Context(), job, data)

	return job
}

// attachRulesResultsParams groups parameters for attaching rules results to template data.
type attachRulesResultsParams struct {
	Request *http.Request
	JobID   string
	Job     *model.Job
	Data    map[string]any
}

func (h *UIHandlers) attachRulesResults(params attachRulesResultsParams) {
	if h.Orchestrator == nil || params.JobID == "" {
		return
	}
	res, err := h.Orchestrator.GetJobResults(params.Request.Context(), params.JobID)
	if err != nil || res == nil {
		return
	}

	params.Data["Results"] = res

	effectiveMode := res.AlertMode
	if normalized, ok := model.ParseSiteAlertMode(string(res.AlertMode)); ok {
		res.AlertMode = normalized
		effectiveMode = normalized
	}
	if !effectiveMode.Valid() {
		if mode := extractSiteAlertMode(params.Data["Site"]); mode.Valid() {
			effectiveMode = mode
		}
	}
	params.Data["RulesResultsView"] = buildRulesResultsView(res, effectiveMode)
}

func extractSiteAlertMode(siteVal any) model.SiteAlertMode {
	switch v := siteVal.(type) {
	case *siteView:
		return alertModeFromSiteView(v)
	case *model.Site:
		if v == nil {
			return ""
		}
		if normalized, ok := model.ParseSiteAlertMode(string(v.AlertMode)); ok {
			return normalized
		}
	}
	return ""
}

func alertModeFromSiteView(view *siteView) model.SiteAlertMode {
	if view == nil {
		return ""
	}
	if normalized, ok := model.ParseSiteAlertMode(string(view.AlertMode)); ok {
		return normalized
	}
	if normalized, ok := model.ParseSiteAlertMode(view.AlertModeStr); ok {
		return normalized
	}
	return ""
}

// attachAlertDelivery attaches alert delivery results (alert jobs) when available.
type attachAlertDeliveryParams struct {
	Request *http.Request
	Job     *model.Job
	Data    map[string]any
}

func (h *UIHandlers) attachAlertDelivery(params attachAlertDeliveryParams) {
	if !h.canAttachAlertDelivery(params) {
		return
	}

	payload := h.loadAlertDeliveryPayload(params.Request.Context(), params.Job.ID)
	if payload == nil {
		return
	}

	h.populateAlertDeliverySinkName(params.Request.Context(), payload)

	params.Data["AlertDelivery"] = buildAlertDeliveryView(payload)
}

func (h *UIHandlers) canAttachAlertDelivery(params attachAlertDeliveryParams) bool {
	if params.Job == nil || params.Job.Type != model.JobTypeAlert || params.Job.ID == "" {
		return false
	}
	if h.JobResults == nil || params.Request == nil {
		return false
	}
	return true
}

func (h *UIHandlers) loadAlertDeliveryPayload(
	ctx context.Context,
	jobID string,
) *service.AlertDeliveryJobResult {
	result, err := h.JobResults.GetByJobID(ctx, jobID)
	if err != nil {
		if !errors.Is(err, data.ErrJobResultsNotFound) &&
			!errors.Is(err, data.ErrJobResultsNotConfigured) {
			h.logger().WarnContext(ctx, "failed to load alert delivery result", "job_id", jobID, "error", err)
		}
		return nil
	}
	if result == nil || result.JobType != model.JobTypeAlert || len(result.Result) == 0 {
		return nil
	}

	var payload service.AlertDeliveryJobResult
	if unmarshalErr := json.Unmarshal(result.Result, &payload); unmarshalErr != nil {
		h.logger().ErrorContext(ctx, "failed to decode alert delivery payload", "job_id", jobID, "error", unmarshalErr)
		return nil
	}

	return &payload
}

func (h *UIHandlers) populateAlertDeliverySinkName(
	ctx context.Context,
	payload *service.AlertDeliveryJobResult,
) {
	if payload.SinkName != "" || h.Sinks == nil || payload.SinkID == "" {
		return
	}

	if sink, err := h.Sinks.GetByID(ctx, payload.SinkID); err == nil && sink != nil {
		payload.SinkName = sink.Name
	}
}

// JobView renders a job detail page with basic job information and rules results if available.
func (h *UIHandlers) JobView(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	data := basePageData(
		r,
		PageMeta{Title: "Merrymaker - Job", PageTitle: "Job Details", CurrentPage: PageJob},
	)
	data["JobID"] = jobID
	job := h.loadJobForView(r, jobID, data)
	h.attachRulesResults(attachRulesResultsParams{
		Request: r,
		JobID:   jobID,
		Job:     job,
		Data:    data,
	})
	h.attachAlertDelivery(attachAlertDeliveryParams{
		Request: r,
		Job:     job,
		Data:    data,
	})

	h.renderDashboardPage(w, r, data)
}

// jobEventFetchParams groups parameters for fetching job events.
type jobEventFetchParams struct {
	JobID        string
	Filters      jobEventFilters
	Limit        int
	Offset       int
	CursorAfter  *string
	CursorBefore *string
}

// fetchJobEvents retrieves events for a job with optional filters.
func (h *UIHandlers) fetchJobEvents(
	ctx context.Context,
	params jobEventFetchParams,
) (*model.EventListPage, error) {
	opts := model.EventListByJobOptions{
		JobID:        params.JobID,
		EventType:    params.Filters.EventType,
		Category:     params.Filters.Category,
		SearchQuery:  params.Filters.SearchQuery,
		SortBy:       params.Filters.SortBy,
		SortDir:      params.Filters.SortDir,
		Limit:        params.Limit,
		Offset:       params.Offset,
		CursorAfter:  params.CursorAfter,
		CursorBefore: params.CursorBefore,
	}
	page, err := h.EventSvc.ListByJob(ctx, opts)
	if err != nil {
		return nil, err
	}
	return page, nil
}

// buildJobEventsPaginationData constructs pagination metadata for job events.
func buildJobEventsPaginationData(params buildJobEventsPaginationParams) PaginationData {
	var startIndex, endIndex int
	if len(params.Events) > 0 {
		startIndex = params.IndexOffset + 1
		endIndex = params.IndexOffset + len(params.Events)
	}

	return PaginationData{
		Page:       params.Page,
		PageSize:   params.PageSize,
		HasPrev:    params.HasPrev,
		HasNext:    params.HasNext,
		StartIndex: startIndex,
		EndIndex:   endIndex,
		TotalCount: params.TotalCount,
		BasePath:   fmt.Sprintf("/jobs/%s/events", params.JobID),
		PrevCursor: params.PrevCursor,
		NextCursor: params.NextCursor,
		PrevIndex:  params.PrevIndexOffset,
		NextIndex:  params.NextIndexOffset,
	}
}

// buildJobEventsPaginationParams groups parameters for building pagination data.
type buildJobEventsPaginationParams struct {
	JobID           string
	Page            int
	PageSize        int
	Events          []*model.Event
	HasNext         bool
	HasPrev         bool
	TotalCount      int
	NextCursor      *string
	PrevCursor      *string
	IndexOffset     int
	NextIndexOffset int
	PrevIndexOffset int
}

// fetchJobEventsCount fetches the total count of events for a job, respecting filters.
func (h *UIHandlers) fetchJobEventsCount(ctx context.Context, params jobEventCountParams) int {
	opts := model.EventListByJobOptions{
		JobID:       params.JobID,
		EventType:   params.Filters.EventType,
		Category:    params.Filters.Category,
		SearchQuery: params.Filters.SearchQuery,
	}
	count, err := h.EventSvc.CountByJob(ctx, opts)
	if err == nil {
		return count
	}
	return 0
}

type jobEventCountParams struct {
	JobID   string
	Filters jobEventFilters
}

type jobEventView struct {
	*model.Event
	DetailsURL  string
	DataTrimmed bool
}

type jobEventsRequest struct {
	jobID       string
	page        int
	pageSize    int
	indexOffset int
	filters     jobEventFilters
	fetchParams jobEventFetchParams
}

func (r jobEventsRequest) usesCursor() bool {
	return r.fetchParams.CursorAfter != nil || r.fetchParams.CursorBefore != nil
}

func parseIndexOffset(raw string) int {
	if raw == "" {
		return 0
	}
	val, err := strconv.Atoi(raw)
	if err != nil || val < 0 {
		return 0
	}
	return val
}

func valueOrDefault(v *string, fallback string) string {
	if v != nil {
		if trimmed := strings.TrimSpace(*v); trimmed != "" {
			return trimmed
		}
	}
	return fallback
}

func parseJobEventsRequest(r *http.Request, jobID string) (jobEventsRequest, error) {
	page, pageSize := getPageParams(r.URL.Query())
	if pageSize == 10 {
		pageSize = defaultEventsPageSize
	}

	q := r.URL.Query()
	cursorAfter := strings.TrimSpace(q.Get("cursor_after"))
	cursorBefore := strings.TrimSpace(q.Get("cursor_before"))
	indexOffset := parseIndexOffset(q.Get("index_offset"))

	if cursorAfter != "" && cursorBefore != "" {
		return jobEventsRequest{}, errors.New("invalid cursor parameters")
	}

	filters := parseJobEventFilters(r)
	fetchParams := jobEventFetchParams{
		JobID:   jobID,
		Filters: filters,
		Limit:   pageSize + 1, // fetch sentinel row for initial page
	}
	if cursorAfter != "" {
		fetchParams.CursorAfter = &cursorAfter
		fetchParams.Limit = pageSize
	}
	if cursorBefore != "" {
		fetchParams.CursorBefore = &cursorBefore
		fetchParams.Limit = pageSize
	}

	return jobEventsRequest{
		jobID:       jobID,
		page:        page,
		pageSize:    pageSize,
		indexOffset: indexOffset,
		filters:     filters,
		fetchParams: fetchParams,
	}, nil
}

func buildJobEventViews(jobID string, events []*model.Event) []jobEventView {
	views := make([]jobEventView, len(events))
	for i, ev := range events {
		if ev == nil {
			continue
		}
		evCopy := *ev
		trimmed := trimEventDataForList(&evCopy)
		views[i] = jobEventView{
			Event:       &evCopy,
			DetailsURL:  fmt.Sprintf("/jobs/%s/events/%s", jobID, evCopy.ID),
			DataTrimmed: trimmed,
		}
	}
	return views
}

// trimEventDataForList removes heavy payload fields (e.g., screenshot images) from list views.
// Returns true when the payload was trimmed.
func trimEventDataForList(ev *model.Event) bool {
	if ev == nil || len(ev.EventData) == 0 {
		return false
	}

	// Screenshots carry large base64 blobs; drop image content for the list fragment.
	if !strings.Contains(strings.ToLower(ev.EventType), "screenshot") {
		return false
	}

	var payload map[string]any
	if err := json.Unmarshal(ev.EventData, &payload); err != nil {
		return false
	}

	if _, ok := payload["image"]; !ok {
		return false
	}
	delete(payload, "image")

	raw, err := json.Marshal(payload)
	if err != nil {
		return false
	}

	ev.EventData = raw
	return true
}

type jobEventsPageState struct {
	Events          []*model.Event
	HasNext         bool
	HasPrev         bool
	NextCursor      *string
	PrevCursor      *string
	NextIndexOffset int
	PrevIndexOffset int
	TotalCount      int
}

func clampPageEvents(events []*model.Event, pageSize int) []*model.Event {
	if len(events) > pageSize {
		return events[:pageSize]
	}
	return events
}

func buildJobEventsPageState(req jobEventsRequest, pageResp *model.EventListPage) jobEventsPageState {
	events := pageResp.Events
	nextCursor := pageResp.NextCursor
	prevCursor := pageResp.PrevCursor

	if req.usesCursor() {
		events = clampPageEvents(events, req.pageSize)
		prevIndexOffset := req.indexOffset - req.pageSize
		if prevIndexOffset < 0 {
			prevIndexOffset = 0
		}
		return jobEventsPageState{
			Events:          events,
			HasNext:         nextCursor != nil,
			HasPrev:         prevCursor != nil,
			NextCursor:      nextCursor,
			PrevCursor:      prevCursor,
			NextIndexOffset: req.indexOffset + len(events),
			PrevIndexOffset: prevIndexOffset,
		}
	}

	return buildOffsetJobEventsPageState(req, events, nextCursor, prevCursor)
}

func buildOffsetJobEventsPageState(
	req jobEventsRequest,
	events []*model.Event,
	nextCursor *string,
	prevCursor *string,
) jobEventsPageState {
	hasPrev := req.page > 1
	hasNext := len(events) > req.pageSize
	events = clampPageEvents(events, req.pageSize)
	if nextCursor == nil && hasNext && len(events) > 0 {
		token, encodeErr := data.EncodeEventCursorFromEvent(
			events[len(events)-1],
			valueOrDefault(req.filters.SortBy, defaultEventSortBy),
			valueOrDefault(req.filters.SortDir, defaultEventSortDir),
		)
		if encodeErr != nil {
			log.Printf("JobEvents: failed to encode next cursor for job %s: %v", req.jobID, encodeErr)
		} else {
			nextCursor = &token
		}
	}

	nextIndexOffset := req.indexOffset + len(events)
	prevIndexOffset := req.indexOffset - req.pageSize
	if prevIndexOffset < 0 {
		prevIndexOffset = 0
	}

	return jobEventsPageState{
		Events:          events,
		HasNext:         hasNext,
		HasPrev:         hasPrev,
		NextCursor:      nextCursor,
		PrevCursor:      prevCursor,
		NextIndexOffset: nextIndexOffset,
		PrevIndexOffset: prevIndexOffset,
	}
}

// JobEvents renders events for a specific job as an HTML fragment for HTMX loading.
func (h *UIHandlers) JobEvents(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" || h.EventSvc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	req, parseErr := parseJobEventsRequest(r, jobID)
	if parseErr != nil {
		http.Error(w, parseErr.Error(), http.StatusBadRequest)
		return
	}

	pageResp, err := h.fetchJobEvents(r.Context(), req.fetchParams)
	if err != nil {
		log.Printf("JobEvents: failed to list events for job %s: %v", jobID, err)
		http.Error(w, "failed to load job events", http.StatusInternalServerError)
		return
	}

	pageState := buildJobEventsPageState(req, pageResp)
	totalCount := h.fetchJobEventsCount(
		r.Context(),
		jobEventCountParams{JobID: jobID, Filters: req.filters},
	)
	if totalCount > 0 && !req.usesCursor() {
		pageState.HasNext = req.indexOffset+len(pageState.Events) < totalCount
	}
	pageState.TotalCount = totalCount

	eventViews := buildJobEventViews(jobID, pageState.Events)

	paginationData := buildJobEventsPaginationData(buildJobEventsPaginationParams{
		JobID:           jobID,
		Page:            req.page,
		PageSize:        req.pageSize,
		Events:          pageState.Events,
		HasNext:         pageState.HasNext,
		HasPrev:         pageState.HasPrev,
		TotalCount:      pageState.TotalCount,
		NextCursor:      pageState.NextCursor,
		PrevCursor:      pageState.PrevCursor,
		IndexOffset:     req.indexOffset,
		NextIndexOffset: pageState.NextIndexOffset,
		PrevIndexOffset: pageState.PrevIndexOffset,
	})
	data := NewTemplateData(r, PageMeta{}).
		WithPagination(paginationData).
		With("Events", eventViews).
		With("JobID", jobID).
		With("Filters", req.filters).
		Build()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if templateErr := h.T.t.ExecuteTemplate(w, "job-events-fragment", data); templateErr != nil {
		log.Printf("JobEvents: template execution failed for job %s: %v", jobID, templateErr)
		http.Error(w, "failed to render job events", http.StatusInternalServerError)
	}
}

// JobEventDetails renders the full event details for a single event (loaded on demand via HTMX).
func (h *UIHandlers) JobEventDetails(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	eventID := r.PathValue("eventId")
	if jobID == "" || eventID == "" || h.EventSvc == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	events, err := h.EventSvc.GetByIDs(r.Context(), []string{eventID})
	if err != nil {
		log.Printf("JobEventDetails: failed to fetch event %s for job %s: %v", eventID, jobID, err)
		http.Error(w, "failed to load event", http.StatusInternalServerError)
		return
	}
	if len(events) == 0 || events[0] == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ev := events[0]
	if ev.SourceJobID == nil || *ev.SourceJobID != jobID {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	data := NewTemplateData(r, PageMeta{}).
		With("Event", ev).
		With("JobID", jobID).
		Build()

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if renderErr := h.T.t.ExecuteTemplate(w, "job-event-details-fragment", data); renderErr != nil {
		log.Printf("JobEventDetails: template execution failed for job %s event %s: %v", jobID, eventID, renderErr)
		http.Error(w, "failed to render event details", http.StatusInternalServerError)
	}
}

// --- Jobs List (Admin) ---

const errMsgUnableLoadJobs = "Unable to load jobs."

// jobsFilter holds the parsed filter parameters for jobs list.
type jobsFilter struct {
	Status    string // "pending", "running", "completed", "failed", "" (all)
	Type      string // "browser", "rules", "alert", "" (all)
	SiteID    string // UUID or "" (all)
	IsTest    string // "true", "false", "" (all)
	SortBy    string // "created_at", "status", "type"
	SortOrder string // "asc", "desc"
}

// parseJobsFilter parses filter parameters from URL query params.
// Supports combined sort parameter (e.g., "created_at:desc") or separate sort/dir params.
func parseJobsFilter(q url.Values) (jobsFilter, error) {
	sortBy, sortOrder := ParseSortParam(q, "sort", "dir")

	return jobsFilter{
		Status:    strings.TrimSpace(q.Get("status")),
		Type:      strings.TrimSpace(q.Get("type")),
		SiteID:    strings.TrimSpace(q.Get("site_id")),
		IsTest:    strings.TrimSpace(q.Get("is_test")),
		SortBy:    sortBy,
		SortOrder: sortOrder,
	}, nil
}

// validateJobStatus returns a pointer to the status if valid, nil otherwise.
func validateJobStatus(s string) *model.JobStatus {
	if s == "" {
		return nil
	}
	switch model.JobStatus(s) {
	case model.JobStatusPending,
		model.JobStatusRunning,
		model.JobStatusCompleted,
		model.JobStatusFailed:
		status := model.JobStatus(s)
		return &status
	}
	return nil
}

// validateJobType returns a pointer to the type if valid, nil otherwise.
func validateJobType(t string) *model.JobType {
	if t == "" {
		return nil
	}
	switch model.JobType(t) {
	case model.JobTypeBrowser, model.JobTypeRules, model.JobTypeAlert, model.JobTypeSecretRefresh:
		jobType := model.JobType(t)
		return &jobType
	}
	return nil
}

// validateSortField returns the field if valid, empty string otherwise.
func validateSortField(field string) string {
	switch field {
	case "created_at", "status", "type":
		return field
	}
	return ""
}

// validateSortOrder returns normalized order if valid, empty string otherwise.
func validateSortOrder(order string) string {
	switch strings.ToLower(order) {
	case SortDirAsc, SortDirDesc:
		return strings.ToLower(order)
	}
	return ""
}

// buildJobListOptions converts filters to JobListOptions with validation.
func buildJobListOptions(filters jobsFilter, limit, offset int) *model.JobListOptions {
	opts := &model.JobListOptions{
		Limit:     limit,
		Offset:    offset,
		Status:    validateJobStatus(filters.Status),
		Type:      validateJobType(filters.Type),
		SortBy:    validateSortField(filters.SortBy),
		SortOrder: validateSortOrder(filters.SortOrder),
	}

	if filters.SiteID != "" {
		opts.SiteID = &filters.SiteID
	}

	if filters.IsTest != "" {
		isTest := filters.IsTest == StrTrue
		opts.IsTest = &isTest
	}

	return opts
}

// fetchJobsWithFilters fetches jobs with optional filtering.
func (h *UIHandlers) fetchJobsWithFilters(
	ctx context.Context,
	filters jobsFilter,
	pg pageOpts,
) ([]*model.JobWithEventCount, error) {
	limit, offset := pg.LimitAndOffset()

	opts := buildJobListOptions(filters, limit, offset)

	jobs, err := h.Jobs.List(ctx, opts)
	if err != nil {
		h.logger().ErrorContext(ctx, "failed to load jobs for UI",
			"error", err,
			"status", filters.Status,
			"type", filters.Type,
			"site_id", filters.SiteID,
			"is_test", filters.IsTest,
			"page", pg.Page,
			"page_size", pg.PageSize,
		)
	}
	return jobs, err
}

// enrichJobsData returns a data enricher that adds filter values and sites to template.
func (h *UIHandlers) enrichJobsData() DataEnricher[*model.JobWithEventCount, jobsFilter] {
	return func(builder *TemplateDataBuilder, _ []*model.JobWithEventCount, filters jobsFilter) {
		builder.
			With("Status", filters.Status).
			With("Type", filters.Type).
			With("SiteID", filters.SiteID).
			With("IsTest", filters.IsTest).
			With("SortBy", filters.SortBy).
			With("SortOrder", filters.SortOrder)

		// Load sites for filter dropdown (best effort)
		if h.SiteSvc != nil && builder.r != nil {
			sites, err := h.SiteSvc.List(builder.r.Context(), MaxSitesForFilter, 0)
			if err != nil {
				h.logger().DebugContext(builder.r.Context(), "failed to load sites for jobs filter dropdown",
					"error", err,
					"max_sites", MaxSitesForFilter,
				)
			} else {
				builder.With("Sites", sites)
			}
		}
	}
}

// JobsList serves the Jobs list page (admin-only), HTMX-aware.
func (h *UIHandlers) JobsList(w http.ResponseWriter, r *http.Request) {
	// Use generic list handler with filtering
	HandleList(ListHandlerOpts[*model.JobWithEventCount, jobsFilter]{
		Handler:         h,
		W:               w,
		R:               r,
		FilteredFetcher: h.fetchJobsWithFilters,
		FilterParser:    parseJobsFilter,
		EnrichData:      h.enrichJobsData(),
		BasePath:        "/jobs",
		PageMeta: PageMeta{
			Title:       "Merrymaker - Jobs",
			PageTitle:   "Jobs",
			CurrentPage: PageJobs,
		},
		ItemsKey:     "Jobs",
		ErrorMessage: errMsgUnableLoadJobs,
		ServiceAvailable: func() bool {
			return h.Jobs != nil
		},
		UnavailableMessage: errMsgUnableLoadJobs,
		UnavailableData: func(builder *TemplateDataBuilder) {
			filters, err := parseJobsFilter(r.URL.Query())
			if err != nil {
				h.logger().WarnContext(r.Context(), "failed to parse jobs filter for unavailable data", "error", err)
				filters = jobsFilter{}
			}
			builder.
				With("Status", filters.Status).
				With("Type", filters.Type).
				With("SiteID", filters.SiteID).
				With("IsTest", filters.IsTest).
				With("SortBy", filters.SortBy).
				With("SortOrder", filters.SortOrder)
		},
	})
}

// renderJobsError renders an error page for jobs list.
func (h *UIHandlers) renderJobsError(w http.ResponseWriter, r *http.Request, msg string) {
	page, pageSize := getPageParams(r.URL.Query())
	filters, err := parseJobsFilter(r.URL.Query())
	if err != nil {
		h.logger().WarnContext(r.Context(), "failed to parse jobs filter for error view", "error", err)
		filters = jobsFilter{}
	}

	data := NewTemplateData(
		r,
		PageMeta{Title: "Merrymaker - Jobs", PageTitle: "Jobs", CurrentPage: PageJobs},
	).
		WithPagination(PaginationData{Page: page, PageSize: pageSize, BasePath: "/jobs"}).
		With("Status", filters.Status).
		With("Type", filters.Type).
		With("SiteID", filters.SiteID).
		With("IsTest", filters.IsTest).
		With("SortBy", filters.SortBy).
		With("SortOrder", filters.SortOrder).
		WithError(msg).
		Build()

	h.renderDashboardPage(w, r, data)
}

// getJobDeleteErrorMessage returns an appropriate error message for job deletion errors.
func getJobDeleteErrorMessage(err error) string {
	switch {
	case errors.Is(err, data.ErrJobNotFound):
		return "Job not found."
	case errors.Is(err, data.ErrJobNotDeletable):
		return "Cannot delete job. Only jobs in pending, completed, or failed status can be deleted."
	case errors.Is(err, data.ErrJobReserved):
		return "Cannot delete job. Job is currently reserved by a worker."
	default:
		return "Unable to delete job. Please try again."
	}
}

// JobDelete handles deleting a job from the UI (admin-only).
func (h *UIHandlers) JobDelete(w http.ResponseWriter, r *http.Request) {
	// Enforce POST method for delete operations (CSRF protection via middleware)
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	id := r.PathValue("id")
	if id == "" || h.Jobs == nil {
		h.renderJobsError(w, r, errMsgUnableLoadJobs)
		return
	}

	h.handleDelete(w, r, deleteHandlerOpts{
		ServiceAvailable: func() bool { return h.Jobs != nil },
		Delete: func(ctx context.Context, jobID string) (bool, error) {
			if err := h.Jobs.Delete(ctx, jobID); err != nil {
				return false, err
			}
			return true, nil
		},
		OnError: func(w http.ResponseWriter, r *http.Request, err error) {
			errMsg := getJobDeleteErrorMessage(err)
			if IsHTMX(r) {
				triggerToast(w, errMsg, "error")
				w.WriteHeader(http.StatusNoContent)
				return
			}
			h.renderJobsError(w, r, errMsg)
		},
		OnSuccess: func(w http.ResponseWriter, r *http.Request, _ bool) {
			if IsHTMX(r) {
				triggerToast(w, "Job deleted successfully", "success")
				w.WriteHeader(http.StatusOK)
				return
			}

			http.Redirect(w, r, "/jobs", http.StatusSeeOther)
		},
	})
}
