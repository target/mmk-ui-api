package httpx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	alertsvm "github.com/target/mmk-ui-api/internal/http/ui/alerts"
	"github.com/target/mmk-ui-api/internal/service"
)

const (
	errMsgUnableLoadAlerts = "Unable to load alerts."
	unknownSiteLabel       = "Unknown Site"
)

// alertsFilter represents filter options for the alerts list view.
type alertsFilter struct {
	SiteID     string
	Severity   string
	RuleType   string
	Unresolved bool
	Sort       string
	Dir        string
}

// parseAlertsFilter extracts filter parameters from URL query.
func parseAlertsFilter(q url.Values) alertsFilter {
	siteID := q.Get("site_id")
	severity := q.Get("severity")
	ruleType := q.Get("rule_type")
	unresolved := q.Get("unresolved") == StrTrue

	sort, dir := ParseSortParam(q, "sort", "dir")

	// Default sort: fired_at desc (newest first)
	if sort == "" {
		sort = "fired_at"
	}
	if dir == "" {
		dir = SortDirDesc
	}

	return alertsFilter{
		SiteID:     siteID,
		Severity:   severity,
		RuleType:   ruleType,
		Unresolved: unresolved,
		Sort:       sort,
		Dir:        dir,
	}
}

// toAlertRowFromAlertWithSiteName converts an AlertWithSiteName to an AlertRow.
// This method is used when the site name is already fetched via JOIN query.
func (h *UIHandlers) toAlertRowFromAlertWithSiteName(
	alertWithSiteName *model.AlertWithSiteName,
) alertsvm.AlertRow {
	isResolved := alertWithSiteName.ResolvedAt != nil

	siteName := strings.TrimSpace(alertWithSiteName.SiteName)
	if siteName == "" {
		siteName = unknownSiteLabel
	}

	siteAlertMode := model.SiteAlertModeActive
	if normalized, ok := model.ParseSiteAlertMode(string(alertWithSiteName.SiteAlertMode)); ok {
		siteAlertMode = normalized
	}

	return alertsvm.AlertRow{
		ID:             alertWithSiteName.ID,
		SiteID:         alertWithSiteName.SiteID,
		SiteName:       siteName,
		SiteAlertMode:  siteAlertMode,
		DeliveryStatus: alertWithSiteName.DeliveryStatus,
		RuleType:       alertWithSiteName.RuleType,
		Severity:       alertWithSiteName.Severity,
		Title:          alertWithSiteName.Title,
		Description:    alertWithSiteName.Description,
		JobDetails:     extractJobDetails(alertWithSiteName.Metadata),
		FiredAt:        alertWithSiteName.FiredAt,
		ResolvedAt:     alertWithSiteName.ResolvedAt,
		ResolvedBy:     alertWithSiteName.ResolvedBy,
		IsResolved:     isResolved,
	}
}

// fetchAlertsWithFiltersAndCount fetches alerts with filtering and total count in a single query.
func (h *UIHandlers) fetchAlertsWithFiltersAndCount(
	ctx context.Context,
	filters alertsFilter,
	pg pageOpts,
) ([]alertsvm.AlertRow, int, error) {
	limit, offset := pg.LimitAndOffset()

	// Build AlertListOptions from filters
	opts := &model.AlertListOptions{
		Limit:      limit,
		Offset:     offset,
		Unresolved: filters.Unresolved,
		Sort:       filters.Sort,
		Dir:        filters.Dir,
	}

	if filters.SiteID != "" {
		opts.SiteID = &filters.SiteID
	}
	if filters.Severity != "" {
		opts.Severity = &filters.Severity
	}
	if filters.RuleType != "" {
		opts.RuleType = &filters.RuleType
	}

	// Fetch alerts with site names and total count in single query using window function
	result, err := h.AlertsSvc.ListWithSiteNamesAndCount(ctx, opts)
	if err != nil {
		h.logger().ErrorContext(ctx, "failed to load alerts for UI",
			"error", err,
			"site_id", filters.SiteID,
			"severity", filters.Severity,
			"rule_type", filters.RuleType,
			"unresolved", filters.Unresolved,
			"page", pg.Page,
			"page_size", pg.PageSize,
		)
		return nil, 0, err
	}

	// Convert to AlertRows (site names already included from JOIN)
	rows := make([]alertsvm.AlertRow, 0, len(result.Alerts))
	for _, alertWithSiteName := range result.Alerts {
		rows = append(rows, h.toAlertRowFromAlertWithSiteName(alertWithSiteName))
	}

	return rows, result.Total, nil
}

// fetchSitesForFilter fetches sites for the filter dropdown (limited to MaxSitesForFilter).
func (h *UIHandlers) fetchSitesForFilter(ctx context.Context) []alertsvm.SiteOption {
	if h.SiteSvc == nil {
		return nil
	}

	sites, err := h.SiteSvc.List(ctx, MaxSitesForFilter, 0)
	if err != nil {
		h.logger().ErrorContext(ctx, "failed to load sites for alert filter", "error", err)
		return nil
	}

	options := make([]alertsvm.SiteOption, 0, len(sites))
	for _, site := range sites {
		options = append(options, alertsvm.SiteOption{
			ID:   site.ID,
			Name: site.Name,
		})
	}

	return options
}

// formatFullTime formats a time.Time into a full display string.
func formatFullTime(t time.Time) string {
	return t.Format("January 2, 2006 at 3:04:05 PM MST")
}

func (h *UIHandlers) buildAlertRowFromAlert(ctx context.Context, alert *model.Alert) alertsvm.AlertRow {
	if alert == nil {
		return alertsvm.AlertRow{}
	}

	rowSource := &model.AlertWithSiteName{
		Alert: *alert,
	}

	if h.SiteSvc != nil && alert.SiteID != "" {
		if site, err := h.SiteSvc.GetByID(ctx, alert.SiteID); err == nil && site != nil {
			rowSource.SiteName = site.Name
			rowSource.SiteAlertMode = site.AlertMode
		}
	}

	return h.toAlertRowFromAlertWithSiteName(rowSource)
}

func (h *UIHandlers) renderAlertRowHTML(row alertsvm.AlertRow) (string, error) {
	if h.T == nil || h.T.t == nil {
		return "", errors.New("template renderer not configured")
	}

	var buf bytes.Buffer
	if err := h.T.t.ExecuteTemplate(&buf, "alerts-row", row); err != nil {
		return "", fmt.Errorf("execute template alerts-row: %w", err)
	}
	return buf.String(), nil
}

// AlertView serves the alert detail page for a specific alert ID, HTMX-aware.
func (h *UIHandlers) AlertView(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || h.AlertsSvc == nil {
		h.NotFound(w, r)
		return
	}

	alert, err := h.AlertsSvc.GetByID(r.Context(), id)
	if err != nil {
		h.logger().ErrorContext(r.Context(), "failed to load alert", "error", err, "alert_id", id)
		h.renderAlertViewError(w, r, errMsgUnableLoadAlerts)
		return
	}
	if alert == nil {
		h.NotFound(w, r)
		return
	}

	h.renderAlertView(w, r, alert)
}

// AlertResolve marks an alert as resolved.
func (h *UIHandlers) AlertResolve(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || h.AlertsSvc == nil {
		h.NotFound(w, r)
		return
	}

	isHTMX := IsHTMX(r)
	target := strings.TrimPrefix(HXTarget(r), "#")
	rowTargetID := "alert-row-" + id
	isRowTarget := isHTMX && target == rowTargetID

	// Get username from session context (set by auth middleware)
	username := "unknown"
	if session := GetSessionFromContext(r.Context()); session != nil {
		username = session.Email
	}

	alert, err := h.AlertsSvc.Resolve(r.Context(), core.ResolveAlertParams{
		ID:         id,
		ResolvedBy: username,
	})
	if err != nil {
		h.alertResolveErrorResponse(alertResolveRequest{
			Writer:    w,
			Request:   r,
			AlertID:   id,
			RowTarget: isRowTarget,
		}, err)
		return
	}

	if alert == nil {
		h.alertResolveErrorResponse(alertResolveRequest{
			Writer:    w,
			Request:   r,
			AlertID:   id,
			RowTarget: isRowTarget,
		}, errors.New("resolve returned nil alert"))
		return
	}

	if isRowTarget {
		h.alertResolveRowSuccess(w, r, alert)
		return
	}

	if isHTMX {
		triggerToast(w, "Alert marked as resolved.", "success")
		HTMX(w).Redirect("/alerts/" + id)
		return
	}

	http.Redirect(w, r, "/alerts/"+id, http.StatusSeeOther)
}

type alertResolveRequest struct {
	Writer    http.ResponseWriter
	Request   *http.Request
	AlertID   string
	RowTarget bool
}

func (h *UIHandlers) alertResolveErrorResponse(req alertResolveRequest, err error) {
	h.logger().ErrorContext(
		req.Request.Context(),
		"failed to resolve alert",
		"error", err,
		"alert_id", req.AlertID,
	)

	if req.RowTarget {
		triggerToast(req.Writer, "Failed to resolve alert.", "error")
		req.Writer.WriteHeader(http.StatusNoContent)
		return
	}

	h.renderAlertViewError(req.Writer, req.Request, "Failed to resolve alert.")
}

func (h *UIHandlers) alertResolveRowSuccess(
	w http.ResponseWriter,
	r *http.Request,
	alert *model.Alert,
) {
	ctx := r.Context()
	row := h.buildAlertRowFromAlert(ctx, alert)
	html, err := h.renderAlertRowHTML(row)
	if err != nil {
		h.logger().ErrorContext(ctx, "failed to render alert row", "error", err, "alert_id", alert.ID)
		triggerToast(w, "Alert resolved, reloading details…", "warning")
		HTMX(w).Redirect("/alerts/" + alert.ID)
		return
	}

	triggerToast(w, "Alert marked as resolved.", "success")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if _, writeErr := w.Write([]byte(html)); writeErr != nil {
		h.logger().ErrorContext(ctx, "failed to write alert row html", "error", writeErr, "alert_id", alert.ID)
	}
}

// buildAlertDetailPage prepares the typed view model for the alert detail page.
func (h *UIHandlers) buildAlertDetailPage(r *http.Request, alert *model.Alert) *alertsvm.DetailPage {
	ctx := r.Context()
	layout := buildLayout(r, PageMeta{
		Title:       "Alert Details - Merrymaker",
		PageTitle:   "Alert Details",
		CurrentPage: PageAlertView,
	})

	if alert == nil {
		h.logger().WarnContext(ctx, "buildAlertDetailPage called with nil alert", "path", r.URL.Path)
		return &alertsvm.DetailPage{
			Layout:       layout,
			Error:        true,
			ErrorMessage: "Alert not found.",
		}
	}

	row := h.buildAlertRowFromAlert(ctx, alert)

	var resolvedAtFormatted string
	if alert.ResolvedAt != nil {
		resolvedAtFormatted = formatFullTime(*alert.ResolvedAt)
	}

	ruleID := ""
	if alert.RuleID != nil {
		ruleID = *alert.RuleID
	}

	deliveries := h.fetchAlertDeliveryJobs(ctx, alert.ID)
	now := time.Now()

	row.DeliveryStatus = deriveAlertDeliveryStatus(row.DeliveryStatus, deliveries)
	jobID := extractJobIDFromContext(alert.EventContext)
	if jobID == "" {
		jobID = extractJobIDFromMetadata(row.JobDetails)
	}

	return &alertsvm.DetailPage{
		Layout:              layout,
		Alert:               &row,
		JobDetails:          row.JobDetails,
		FiredAtFormatted:    formatFullTime(alert.FiredAt),
		ResolvedAtFormatted: resolvedAtFormatted,
		EventContextDisplay: formatJSONForDisplay(alert.EventContext),
		MetadataDisplay:     formatJSONForDisplay(alert.Metadata),
		JobID:               jobID,
		RuleID:              ruleID,
		Deliveries:          deliveries,
		DeliveryStats:       computeAlertDeliveryStats(ctx, h.logger(), deliveries),
		SinksConfigured:     h.countHTTPAlertSinks(ctx) > 0,
		JobsMayBeReaped:     now.After(alert.CreatedAt.Add(24 * time.Hour)),
	}
}

// extractJobDetails flattens alert metadata and returns job-related key/value pairs for display.
func extractJobDetails(meta json.RawMessage) []alertsvm.MetadataDetail {
	flat := flattenMetadata(meta)
	if len(flat) == 0 {
		return nil
	}

	details := make([]alertsvm.MetadataDetail, 0, len(flat))
	for key, value := range flat {
		if !isJobMetadataKey(key) {
			continue
		}
		formatted := formatMetadataValue(value)
		if formatted == "" {
			continue
		}
		details = append(details, alertsvm.MetadataDetail{
			Label: humanizeMetadataKey(key),
			Value: formatted,
		})
	}

	if len(details) == 0 {
		return nil
	}

	sort.Slice(details, func(i, j int) bool {
		pi := jobMetadataPriority(details[i].Label)
		pj := jobMetadataPriority(details[j].Label)
		if pi == pj {
			return details[i].Label < details[j].Label
		}
		return pi < pj
	})

	return details
}

// flattenMetadata converts nested metadata into a flat key/value map using dot notation.
func flattenMetadata(meta json.RawMessage) map[string]any {
	if len(meta) == 0 {
		return nil
	}

	trimmed := bytes.TrimSpace(meta)
	if !json.Valid(trimmed) || bytes.Equal(trimmed, []byte("null")) || bytes.Equal(trimmed, []byte("{}")) {
		return nil
	}

	var raw map[string]any
	if err := json.Unmarshal(trimmed, &raw); err != nil || len(raw) == 0 {
		return nil
	}

	out := make(map[string]any, len(raw))
	var walk func(map[string]any, string)

	walk = func(m map[string]any, prefix string) {
		for k, v := range m {
			key := k
			if prefix != "" {
				key = prefix + "." + k
			}
			switch typed := v.(type) {
			case map[string]any:
				walk(typed, key)
			default:
				out[key] = typed
			}
		}
	}

	walk(raw, "")

	return out
}

// isJobMetadataKey reports whether the key appears related to job attribution.
func isJobMetadataKey(key string) bool {
	tokens := tokenizeMetadataKey(key)
	if len(tokens) == 0 {
		return false
	}

	jobLike := map[string]struct{}{
		"job":     {},
		"session": {},
		"batch":   {},
		"run":     {},
	}

	for i, token := range tokens {
		root := strings.TrimSuffix(token, "id")
		if _, ok := jobLike[root]; ok && root != token {
			return true
		}
		if token == "id" && i > 0 {
			if _, ok := jobLike[tokens[i-1]]; ok {
				return true
			}
		}
		if _, ok := jobLike[token]; ok && i+1 < len(tokens) && tokens[i+1] == "id" {
			return true
		}
	}

	return false
}

// humanizeMetadataKey converts a metadata key into a display-friendly label.
func humanizeMetadataKey(key string) string {
	replacer := strings.NewReplacer("_", " ", ".", " ")
	words := strings.Fields(replacer.Replace(strings.TrimSpace(key)))
	for i, word := range words {
		if len(word) == 0 {
			continue
		}
		words[i] = strings.ToUpper(word[:1]) + strings.ToLower(word[1:])
	}
	return strings.Join(words, " ")
}

// formatMetadataValue renders supported metadata values as strings for display.
func formatMetadataValue(value any) string {
	switch v := value.(type) {
	case string:
		return strings.TrimSpace(v)
	case float64, bool, int, int64:
		return fmt.Sprint(v)
	case []any:
		parts := make([]string, 0, len(v))
		for _, item := range v {
			if formatted := formatMetadataValue(item); formatted != "" {
				parts = append(parts, formatted)
			}
		}
		return strings.Join(parts, ", ")
	}
	return ""
}

// tokenizeMetadataKey splits a metadata key into lowercase tokens on non-alphanumeric boundaries.
func tokenizeMetadataKey(key string) []string {
	if strings.TrimSpace(key) == "" {
		return nil
	}
	lower := strings.ToLower(strings.TrimSpace(key))
	return strings.FieldsFunc(lower, func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	})
}

// jobMetadataPriority assigns an ordering weight so identifiers appear before descriptive fields.
func jobMetadataPriority(label string) int {
	lower := strings.ToLower(strings.TrimSpace(label))
	priorities := map[string]int{
		"job id":     0,
		"session id": 1,
		"batch id":   2,
		"run id":     3,
	}
	if p, ok := priorities[lower]; ok {
		return p
	}
	return 100 // default priority
}

// renderAlertView renders the alert detail view with full context.
func (h *UIHandlers) renderAlertView(w http.ResponseWriter, r *http.Request, alert *model.Alert) {
	page := h.buildAlertDetailPage(r, alert)
	h.renderDashboardPage(w, r, page)
}

// renderAlertViewError renders an error page for alert view.
func (h *UIHandlers) renderAlertViewError(w http.ResponseWriter, r *http.Request, msg string) {
	page := &alertsvm.DetailPage{
		Layout: buildLayout(r, PageMeta{
			Title:       "Alert Details - Merrymaker",
			PageTitle:   "Alert Details",
			CurrentPage: PageAlertView,
		}),
		Error:        true,
		ErrorMessage: msg,
	}

	h.renderDashboardPage(w, r, page)
}

// formatJSONForDisplay formats JSON bytes for human-readable display.
func formatJSONForDisplay(data json.RawMessage) string {
	if len(data) == 0 || string(data) == "{}" || string(data) == "null" {
		return ""
	}

	var formatted map[string]any
	if err := json.Unmarshal(data, &formatted); err != nil {
		return string(data)
	}

	pretty, err := json.MarshalIndent(formatted, "", "  ")
	if err != nil {
		return string(data)
	}

	return string(pretty)
}

// extractJobIDFromContext attempts to extract a job_id from event context JSON.
func extractJobIDFromContext(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}

	var ctx map[string]any
	if err := json.Unmarshal(data, &ctx); err != nil {
		return ""
	}

	if jobID, ok := ctx["job_id"].(string); ok {
		return jobID
	}

	return ""
}

// extractJobIDFromMetadata attempts to find a job identifier from parsed metadata details.
func extractJobIDFromMetadata(details []alertsvm.MetadataDetail) string {
	for _, detail := range details {
		if strings.TrimSpace(detail.Value) == "" {
			continue
		}
		if strings.Contains(strings.ToLower(detail.Label), "job") {
			return detail.Value
		}
	}
	return ""
}

// fetchAlertDeliveryJobs queries alert delivery jobs for a given alert ID.
func (h *UIHandlers) fetchAlertDeliveryJobs(
	ctx context.Context,
	alertID string,
) []alertsvm.DeliveryRow {
	if alertID == "" {
		return nil
	}

	rowsByJobID := h.collectAlertDeliveryRows(ctx, alertID)

	if len(rowsByJobID) == 0 {
		return nil
	}

	return sortAlertDeliveries(rowsByJobID)
}

// queryAlertJobs fetches all alert-type jobs from the database.
// TODO: This is inefficient as it fetches all alert jobs and filters in-process.
// Consider adding AlertID filtering at the repository level for better performance.
func (h *UIHandlers) queryAlertJobs(
	ctx context.Context,
	alertID string,
) []*model.JobWithEventCount {
	alertType := model.JobTypeAlert
	jobs, err := h.Jobs.List(ctx, &model.JobListOptions{
		Type:      &alertType,
		Limit:     2000, // Increased limit for safety until proper filtering is implemented
		SortBy:    "created_at",
		SortOrder: "desc", // Get newest jobs first
	})
	if err != nil {
		h.logger().ErrorContext(ctx, "failed to query alert delivery jobs", "error", err, "alert_id", alertID)
		return nil
	}
	return jobs
}

func (h *UIHandlers) collectAlertDeliveryRows(ctx context.Context, alertID string) map[string]alertsvm.DeliveryRow {
	rowsByJobID := make(map[string]alertsvm.DeliveryRow)
	h.mergePersistedDeliveries(ctx, alertID, rowsByJobID)
	h.mergeLiveDeliveries(ctx, alertID, rowsByJobID)
	return rowsByJobID
}

func (h *UIHandlers) mergePersistedDeliveries(
	ctx context.Context,
	alertID string,
	rows map[string]alertsvm.DeliveryRow,
) {
	if h.JobResults == nil {
		return
	}
	results, err := h.JobResults.ListByAlertID(ctx, alertID)
	if err != nil {
		h.logger().ErrorContext(ctx, "failed to query alert delivery results", "error", err, "alert_id", alertID)
		return
	}
	for _, jr := range results {
		if delivery, ok := h.buildDeliveryRowFromResult(ctx, jr); ok {
			rows[delivery.JobID] = delivery
		}
	}
}

func (h *UIHandlers) mergeLiveDeliveries(ctx context.Context, alertID string, rows map[string]alertsvm.DeliveryRow) {
	if h.Jobs == nil {
		return
	}
	jobs := h.queryAlertJobs(ctx, alertID)
	for _, jobWithCount := range jobs {
		delivery, ok := h.buildDeliveryRow(ctx, jobWithCount.Job, alertID)
		if !ok || delivery.JobID == "" {
			continue
		}
		if existing, found := rows[delivery.JobID]; found && isTerminalAlertStatus(existing.Status) {
			continue
		}
		rows[delivery.JobID] = delivery
	}
}

func sortAlertDeliveries(rows map[string]alertsvm.DeliveryRow) []alertsvm.DeliveryRow {
	deliveries := make([]alertsvm.DeliveryRow, 0, len(rows))
	for _, delivery := range rows {
		deliveries = append(deliveries, delivery)
	}

	sort.Slice(deliveries, func(i, j int) bool {
		return deliveries[i].AttemptedAt.After(deliveries[j].AttemptedAt)
	})

	return deliveries
}

func isTerminalAlertStatus(status string) bool {
	return status == string(model.JobStatusCompleted) || status == string(model.JobStatusFailed)
}

func computeAlertDeliveryStats(
	ctx context.Context,
	logger *slog.Logger,
	deliveries []alertsvm.DeliveryRow,
) alertsvm.DeliveryStats {
	stats := alertsvm.DeliveryStats{
		Total: len(deliveries),
	}

	for _, delivery := range deliveries {
		switch delivery.Status {
		case string(model.JobStatusCompleted):
			stats.Completed++
		case string(model.JobStatusFailed):
			stats.Failed++
		case string(model.JobStatusRunning):
			stats.Running++
		case string(model.JobStatusPending):
			stats.Pending++
		default:
			stats.Other++
			if strings.TrimSpace(delivery.Status) != "" {
				logger.WarnContext(ctx, "unknown alert delivery status encountered",
					"status", delivery.Status,
					"job_id", delivery.JobID,
				)
			} else {
				logger.WarnContext(ctx, "empty alert delivery status encountered", "job_id", delivery.JobID)
			}
		}
	}

	return stats
}

// deriveAlertDeliveryStatus infers the alert delivery status from delivery attempts when available.
func deriveAlertDeliveryStatus(
	initial model.AlertDeliveryStatus,
	deliveries []alertsvm.DeliveryRow,
) model.AlertDeliveryStatus {
	if initial == model.AlertDeliveryStatusMuted {
		return initial
	}

	hasCompleted := false
	hasFailed := false
	hasRunning := false
	hasPending := false

	for _, delivery := range deliveries {
		switch delivery.Status {
		case string(model.JobStatusCompleted):
			hasCompleted = true
		case string(model.JobStatusFailed):
			hasFailed = true
		case string(model.JobStatusRunning):
			hasRunning = true
		case string(model.JobStatusPending):
			hasPending = true
		}
	}

	switch {
	case hasCompleted:
		return model.AlertDeliveryStatusDispatched
	case hasRunning, hasPending:
		return model.AlertDeliveryStatusPending
	case hasFailed:
		return model.AlertDeliveryStatusFailed
	default:
		return initial
	}
}

// buildDeliveryRow constructs an alertDeliveryRow from a job if it matches the alert ID.
func (h *UIHandlers) buildDeliveryRow(
	ctx context.Context,
	job model.Job,
	alertID string,
) (alertsvm.DeliveryRow, bool) {
	var payload struct {
		SinkID  string `json:"sink_id"`
		Payload struct {
			ID string `json:"id"`
		} `json:"payload"`
	}
	if err := json.Unmarshal(job.Payload, &payload); err != nil || payload.Payload.ID != alertID {
		return alertsvm.DeliveryRow{}, false
	}

	sinkName := "Unknown Sink"
	if sink, err := h.Sinks.GetByID(ctx, payload.SinkID); err == nil && sink != nil {
		sinkName = sink.Name
	}

	// Use StartedAt if available (actual attempt time), otherwise fall back to ScheduledAt
	attemptedAt := job.ScheduledAt
	if job.StartedAt != nil {
		attemptedAt = *job.StartedAt
	}

	delivery := alertsvm.DeliveryRow{
		SinkID:        payload.SinkID,
		SinkName:      sinkName,
		JobID:         job.ID,
		Status:        string(job.Status),
		AttemptNumber: job.RetryCount + 1,
		AttemptedAt:   attemptedAt,
		CompletedAt:   job.CompletedAt,
	}

	if job.LastError != nil && *job.LastError != "" {
		delivery.ErrorMessage = *job.LastError
	}

	return delivery, true
}

// buildDeliveryRowFromResult converts a persisted job result into an alertDeliveryRow.
func (h *UIHandlers) buildDeliveryRowFromResult(
	ctx context.Context,
	result *model.JobResult,
) (alertsvm.DeliveryRow, bool) {
	if result == nil || len(result.Result) == 0 {
		return alertsvm.DeliveryRow{}, false
	}

	payload, ok := h.unmarshalDeliveryPayload(ctx, result)
	if !ok {
		return alertsvm.DeliveryRow{}, false
	}

	delivery := h.buildDeliveryRowFromPayload(result, payload)
	h.enrichDeliveryWithSinkName(ctx, &delivery, payload.SinkID)

	return delivery, true
}

// unmarshalDeliveryPayload unmarshals the job result into an AlertDeliveryJobResult.
func (h *UIHandlers) unmarshalDeliveryPayload(
	ctx context.Context,
	result *model.JobResult,
) (service.AlertDeliveryJobResult, bool) {
	var payload service.AlertDeliveryJobResult
	if err := json.Unmarshal(result.Result, &payload); err != nil {
		jobID := "<nil>"
		if result.JobID != nil {
			jobID = *result.JobID
		}
		h.logger().ErrorContext(ctx, "failed to decode alert delivery result", "error", err, "job_id", jobID)
		return payload, false
	}
	return payload, true
}

// buildDeliveryRowFromPayload constructs an alertDeliveryRow from the payload.
func (h *UIHandlers) buildDeliveryRowFromPayload(
	result *model.JobResult,
	payload service.AlertDeliveryJobResult,
) alertsvm.DeliveryRow {
	attemptedAt := payload.AttemptedAt
	if attemptedAt.IsZero() {
		attemptedAt = result.CreatedAt
	}

	jobID := ""
	if result.JobID != nil {
		jobID = *result.JobID
	}

	return alertsvm.DeliveryRow{
		SinkID:        payload.SinkID,
		SinkName:      payload.SinkName,
		JobID:         jobID,
		Status:        string(payload.JobStatus),
		AttemptNumber: payload.AttemptNumber,
		ErrorMessage:  payload.ErrorMessage,
		AttemptedAt:   attemptedAt,
		CompletedAt:   payload.CompletedAt,
	}
}

// enrichDeliveryWithSinkName populates the SinkName if missing.
func (h *UIHandlers) enrichDeliveryWithSinkName(ctx context.Context, delivery *alertsvm.DeliveryRow, sinkID string) {
	if delivery.SinkName == "" && h.Sinks != nil && sinkID != "" {
		if sink, err := h.Sinks.GetByID(ctx, sinkID); err == nil && sink != nil {
			delivery.SinkName = sink.Name
		}
	}
}

// countHTTPAlertSinks returns the total number of configured HTTP alert sinks.
func (h *UIHandlers) countHTTPAlertSinks(ctx context.Context) int {
	if h.Sinks == nil {
		return 0
	}
	sinks, err := h.Sinks.List(ctx, 1, 0)
	if err != nil || len(sinks) == 0 {
		return 0
	}
	// If we got at least one sink, there are sinks configured
	return 1
}
