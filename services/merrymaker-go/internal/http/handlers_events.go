// Package httpx provides HTTP handlers and utilities for the merrymaker job system API.
package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/google/uuid"
)

// EventHandlers provides HTTP handlers for event-related operations.
type EventHandlers struct {
	Svc    *service.EventService
	Filter *service.EventFilterService
	// Optional: Jobs service to enqueue rules jobs after ingestion (best-effort).
	Jobs *service.JobService
	// Optional: Orchestrator for rules job enqueue with dedupe
	Orchestrator *service.RulesOrchestrationService
	// Optional: Sites service to derive scope for rules jobs
	Sites *service.SiteService
	// Configuration for rules engine integration
	AutoEnqueueRules bool
	// Optional: structured logger for request-scoped logging
	Logger *slog.Logger
}

// EventHandlersOptions configures event handlers.
type EventHandlersOptions struct {
	EventService     *service.EventService
	FilterService    *service.EventFilterService
	JobService       *service.JobService
	Orchestrator     *service.RulesOrchestrationService
	SiteService      *service.SiteService
	AutoEnqueueRules bool
	Logger           *slog.Logger
}

// NewEventHandlers constructs EventHandlers with explicit dependency injection.
func NewEventHandlers(opts EventHandlersOptions) *EventHandlers {
	return &EventHandlers{
		Svc:              opts.EventService,
		Filter:           opts.FilterService,
		Jobs:             opts.JobService,
		Orchestrator:     opts.Orchestrator,
		Sites:            opts.SiteService,
		AutoEnqueueRules: opts.AutoEnqueueRules,
		Logger:           opts.Logger,
	}
}

func (h *EventHandlers) logger() *slog.Logger {
	if h != nil && h.Logger != nil {
		return h.Logger
	}
	return slog.Default()
}

// Re-export Puppeteer types from types package for convenience.
type (
	// PuppeteerEventBatch re-exports model.PuppeteerEventBatch for convenience.
	PuppeteerEventBatch = model.PuppeteerEventBatch
	// PuppeteerEvent re-exports model.PuppeteerEvent for convenience.
	PuppeteerEvent = model.PuppeteerEvent
	// PuppeteerEventParams re-exports model.PuppeteerEventParams for convenience.
	PuppeteerEventParams = model.PuppeteerEventParams
	// PuppeteerAttribution re-exports model.PuppeteerAttribution for convenience.
	PuppeteerAttribution = model.PuppeteerAttribution
	// PuppeteerEventMetadata re-exports model.PuppeteerEventMetadata for convenience.
	PuppeteerEventMetadata = model.PuppeteerEventMetadata
	// PuppeteerBatchMetadata re-exports model.PuppeteerBatchMetadata for convenience.
	PuppeteerBatchMetadata = model.PuppeteerBatchMetadata
	// PuppeteerChecksumInfo re-exports model.PuppeteerChecksumInfo for convenience.
	PuppeteerChecksumInfo = model.PuppeteerChecksumInfo
	// PuppeteerSequenceInfo re-exports model.PuppeteerSequenceInfo for convenience.
	PuppeteerSequenceInfo = model.PuppeteerSequenceInfo
)

// BulkInsert handles HTTP requests to insert multiple events in bulk.
func (h *EventHandlers) BulkInsert(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	var batch PuppeteerEventBatch
	if !DecodeJSON(w, r, &batch) {
		return
	}

	bulkReq, err := h.processEventBatch(&batch, w)
	if err != nil {
		return // Error already written to response
	}

	filterStats, shouldProcessMap, err := h.applyEventFiltering(ctx, bulkReq, w)
	if err != nil {
		return // Error already written to response
	}

	count, err := h.insertEventsWithFlags(ctx, bulkReq, shouldProcessMap)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "insert_failed", Err: err})
		return
	}

	rulesJobID := h.handleRulesJobEnqueue(ctx, bulkReq, filterStats)
	response := map[string]any{
		"inserted":    count,
		"batch_id":    batch.BatchID,
		"session_id":  batch.SessionID,
		"event_count": len(batch.Events),
	}
	if rulesJobID != nil {
		response["rules_job_id"] = *rulesJobID
	}
	WriteJSON(w, http.StatusOK, response)
}

// processEventBatch processes and validates the event batch.
func (h *EventHandlers) processEventBatch(
	batch *PuppeteerEventBatch,
	w http.ResponseWriter,
) (*model.BulkEventRequest, error) {
	if batch == nil {
		errBatchRequired := errors.New("event batch is required")
		WriteError(
			w,
			ErrorParams{
				Code:    http.StatusBadRequest,
				ErrCode: "transformation_failed",
				Err:     errBatchRequired,
			},
		)
		return nil, errBatchRequired
	}

	bulkReq, err := h.transformPuppeteerBatch(batch)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "transformation_failed", Err: err})
		return nil, err
	}

	if validationErr := bulkReq.Validate(h.Svc.GetConfig().MaxBatch); validationErr != nil {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "validation_failed", Err: validationErr},
		)
		return nil, validationErr
	}

	return bulkReq, nil
}

// applyEventFiltering applies event filtering and returns statistics.
func (h *EventHandlers) applyEventFiltering(
	ctx context.Context,
	bulkReq *model.BulkEventRequest,
	w http.ResponseWriter,
) (*service.EventFilterStats, map[int]bool, error) {
	if h.Filter == nil {
		WriteError(
			w,
			ErrorParams{
				Code:    http.StatusInternalServerError,
				ErrCode: "filter_not_configured",
				Err:     errors.New("event filter is not configured"),
			},
		)
		return nil, nil, errors.New("filter not configured")
	}

	shouldProcessMap := h.Filter.ShouldProcessEvents(bulkReq.Events)
	// Derive stats from shouldProcessMap without re-scanning events
	total := len(bulkReq.Events)
	processable := 0
	for i := range bulkReq.Events {
		if shouldProcessMap[i] {
			processable++
		}
	}
	filtered := total - processable
	filterRatio := 0.0
	if total > 0 {
		filterRatio = float64(filtered) / float64(total) * 100.0
	}
	filterStats := service.EventFilterStats{
		TotalEvents:       total,
		ProcessableEvents: processable,
		FilteredEvents:    filtered,
		FilterRatio:       filterRatio,
	}

	h.logger().DebugContext(ctx, "event filtering applied",
		"total_events", filterStats.TotalEvents,
		"processable_events", filterStats.ProcessableEvents,
		"filtered_events", filterStats.FilteredEvents,
		"filter_ratio", filterStats.FilterRatio)

	return &filterStats, shouldProcessMap, nil
}

// insertEventsWithFlags inserts events with processing flags.
func (h *EventHandlers) insertEventsWithFlags(
	ctx context.Context,
	bulkReq *model.BulkEventRequest,
	shouldProcessMap map[int]bool,
) (int, error) {
	count, err := h.Svc.BulkInsertWithProcessingFlags(ctx, *bulkReq, shouldProcessMap)
	if err != nil {
		return 0, err
	}
	return count, nil
}

// handleRulesJobEnqueue handles rules job enqueuing if enabled.
func (h *EventHandlers) handleRulesJobEnqueue(
	ctx context.Context,
	bulkReq *model.BulkEventRequest,
	filterStats *service.EventFilterStats,
) *string {
	if !h.AutoEnqueueRules || bulkReq.SourceJobID == nil || *bulkReq.SourceJobID == "" ||
		filterStats.ProcessableEvents == 0 {
		return nil
	}

	rulesJobID := h.enqueueRulesJobBestEffort(ctx, *bulkReq.SourceJobID)
	if rulesJobID != nil {
		h.logger().DebugContext(ctx, "rules job enqueued successfully",
			"source_job_id", *bulkReq.SourceJobID,
			"rules_job_id", *rulesJobID,
			"processable_events", filterStats.ProcessableEvents)
	} else {
		h.logger().DebugContext(ctx, "rules job enqueue failed or skipped",
			"source_job_id", *bulkReq.SourceJobID,
			"processable_events", filterStats.ProcessableEvents)
	}

	return rulesJobID
}

// transformPuppeteerBatch converts a puppeteer event batch to the job service format.
func (h *EventHandlers) transformPuppeteerBatch(batch *PuppeteerEventBatch) (*model.BulkEventRequest, error) {
	if batch == nil {
		return nil, errors.New("event batch is required")
	}

	events := make([]model.RawEvent, 0, len(batch.Events))

	for i := range batch.Events {
		event := &batch.Events[i]
		// Convert payload to JSON
		payloadBytes, err := json.Marshal(event.Params.Payload)
		if err != nil {
			return nil, err
		}

		// Create metadata map
		metadata := map[string]any{
			"method":           event.Method,
			"category":         event.Metadata.Category,
			"tags":             event.Metadata.Tags,
			"processing_hints": event.Metadata.ProcessingHints,
			"sequence_number":  event.Metadata.SequenceNumber,
			"attribution":      event.Params.Attribution,
		}

		// Determine priority from processing hints
		priority := 0
		if hints, ok := event.Metadata.ProcessingHints["isHighPriority"]; ok {
			if isHigh, isBool := hints.(bool); isBool && isHigh {
				priority = 75
			}
		}

		metadataBytes, err := json.Marshal(metadata)
		if err != nil {
			return nil, err
		}

		rawEvent := model.RawEvent{
			Type:      event.Method,
			Data:      json.RawMessage(payloadBytes),
			Timestamp: time.Unix(0, event.Params.Timestamp*int64(time.Millisecond)),
			Metadata:  metadataBytes,
			Priority:  &priority,
		}

		events = append(events, rawEvent)
	}

	// Build bulk event request and propagate job linkage when provided
	req := &model.BulkEventRequest{
		SessionID: batch.SessionID,
		Events:    events,
	}
	if batch.BatchMetadata.JobID != "" {
		req.SourceJobID = &batch.BatchMetadata.JobID
	}
	return req, nil
}

type eventListResponse struct {
	Events     []*model.Event `json:"events"`
	NextCursor *string        `json:"next_cursor,omitempty"`
	PrevCursor *string        `json:"prev_cursor,omitempty"`
}

func parseCursorParams(r *http.Request) (*string, *string, error) {
	q := r.URL.Query()
	cursor := strings.TrimSpace(q.Get("cursor"))
	if cursor == "" {
		return nil, nil, nil
	}

	dir := strings.ToLower(strings.TrimSpace(q.Get("dir")))
	if dir == "prev" || dir == "before" {
		return nil, &cursor, nil
	}

	if dir == "" || dir == "next" || dir == "after" {
		return &cursor, nil, nil
	}

	return nil, nil, fmt.Errorf("invalid dir value: %s", dir)
}

// ListByJob handles HTTP requests to list events by job ID with pagination.
func (h *EventHandlers) ListByJob(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("job id is required")},
		)
		return
	}
	if _, err := uuid.Parse(jobID); err != nil {
		WriteError(
			w,
			ErrorParams{
				Code:    http.StatusBadRequest,
				ErrCode: "invalid_path",
				Err:     errors.New("job id must be a valid UUID"),
			},
		)
		return
	}
	limit := parseIntQuery(r, "limit", -1)
	offset := parseIntQuery(r, "offset", -1)
	cursorAfter, cursorBefore, cursorErr := parseCursorParams(r)
	if cursorErr != nil {
		WriteError(
			w,
			ErrorParams{
				Code:    http.StatusBadRequest,
				ErrCode: "invalid_query",
				Err:     cursorErr,
			},
		)
		return
	}

	if cursorAfter != nil || cursorBefore != nil {
		offset = 0
	}

	opts := model.EventListByJobOptions{
		JobID:        jobID,
		Limit:        limit,
		Offset:       offset,
		CursorAfter:  cursorAfter,
		CursorBefore: cursorBefore,
	}
	page, err := h.Svc.ListByJob(r.Context(), opts)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "list_failed", Err: err})
		return
	}
	WriteJSON(w, http.StatusOK, eventListResponse{
		Events:     page.Events,
		NextCursor: page.NextCursor,
		PrevCursor: page.PrevCursor,
	})
}

// enqueueRulesJobBestEffort selects unprocessed, should-process events for the source job and enqueues a rules job.
func (h *EventHandlers) enqueueRulesJobBestEffort(ctx context.Context, sourceJobID string) *string {
	siteID, ok := h.getSiteIDForJob(ctx, sourceJobID)
	if !ok {
		h.logger().DebugContext(ctx, "rules enqueue skipped: no site id for job", "job_id", sourceJobID)
		return nil
	}
	ids := h.getProcessableEventIDs(ctx, sourceJobID)
	if len(ids) == 0 {
		h.logger().DebugContext(ctx, "rules enqueue skipped: no processable events", "job_id", sourceJobID)
		return nil
	}
	return h.createRulesJob(ctx, rulesJobEnqueueParams{SourceJobID: sourceJobID, SiteID: siteID, EventIDs: ids})
}

func (h *EventHandlers) getSiteIDForJob(ctx context.Context, jobID string) (string, bool) {
	if h.Jobs == nil {
		h.logger().DebugContext(ctx, "rules enqueue skipped: jobs service not configured")
		return "", false
	}
	job, err := h.Jobs.GetByID(ctx, jobID)
	if err != nil || job == nil || job.SiteID == nil || *job.SiteID == "" {
		h.logger().DebugContext(
			ctx,
			"rules enqueue skipped: job not found or no site id",
			"job_id",
			jobID,
			"error",
			err,
		)
		return "", false
	}
	return *job.SiteID, true
}

const maxRulesJobIDs = 500

func (h *EventHandlers) getProcessableEventIDs(ctx context.Context, jobID string) []string {
	page, err := h.Svc.ListByJob(ctx, model.EventListByJobOptions{JobID: jobID, Limit: 1000, Offset: 0})
	if err != nil || len(page.Events) == 0 {
		h.logger().DebugContext(
			ctx,
			"rules enqueue: no events for job or list failed",
			"job_id",
			jobID,
			"error",
			err,
		)
		return nil
	}
	ids := make([]string, 0, len(page.Events))
	for _, e := range page.Events {
		if e.ShouldProcess && !e.Processed {
			ids = append(ids, e.ID)
		}
		if len(ids) >= maxRulesJobIDs {
			break // soft cap to keep payloads reasonable
		}
	}
	return ids
}

type rulesJobEnqueueParams struct {
	SourceJobID string
	SiteID      string
	EventIDs    []string
}

func (h *EventHandlers) createRulesJob(ctx context.Context, p rulesJobEnqueueParams) *string {
	scope, isTest := h.resolveScopeAndIsTest(ctx, p.SourceJobID)
	return h.enqueueRules(ctx, &rulesEnqueueInput{p: p, scope: scope, isTest: isTest})
}

// rulesEnqueueInput aggregates inputs to keep param count small.
type rulesEnqueueInput struct {
	p      rulesJobEnqueueParams
	scope  string
	isTest bool
}

// resolveScopeFromJobSite returns the scope derived from the job's associated Site, or "default".
func (h *EventHandlers) resolveScopeFromJobSite(ctx context.Context, j *model.Job) string {
	scope := DefaultScope
	if j == nil || j.SiteID == nil || *j.SiteID == "" || h.Sites == nil {
		return scope
	}
	site, err := h.Sites.GetByID(ctx, *j.SiteID)
	if err != nil || site == nil || site.Scope == nil || *site.Scope == "" {
		return scope
	}
	return *site.Scope
}

func (h *EventHandlers) resolveScopeAndIsTest(ctx context.Context, sourceJobID string) (string, bool) {
	scope := DefaultScope
	isTest := false
	if h.Jobs == nil || sourceJobID == "" {
		return scope, isTest
	}
	j, err := h.Jobs.GetByID(ctx, sourceJobID)
	if err != nil || j == nil {
		if err != nil {
			h.logger().DebugContext(
				ctx,
				"resolve scope: failed to load source job",
				"job_id",
				sourceJobID,
				"error",
				err,
			)
		}
		return scope, isTest
	}
	isTest = j.IsTest
	scope = h.resolveScopeFromJobSite(ctx, j)
	return scope, isTest
}

func (h *EventHandlers) enqueueRules(ctx context.Context, in *rulesEnqueueInput) *string {
	if in == nil {
		return nil
	}
	if jobID := h.enqueueRulesWithOrchestrator(ctx, in); jobID != nil {
		return jobID
	}
	return h.enqueueRulesWithJobsService(ctx, in)
}

func (h *EventHandlers) enqueueRulesWithOrchestrator(ctx context.Context, in *rulesEnqueueInput) *string {
	if in == nil || h.Orchestrator == nil {
		return nil
	}

	job, err := h.Orchestrator.EnqueueRulesProcessingJob(ctx, service.EnqueueRulesJobRequest{
		EventIDs: in.p.EventIDs,
		SiteID:   in.p.SiteID,
		Scope:    in.scope,
		Priority: 50,
		IsTest:   in.isTest,
	})
	if err == nil && job != nil {
		h.logger().DebugContext(
			ctx,
			"rules enqueue: job created (orchestrator)",
			"job_id",
			job.ID,
			"events",
			len(in.p.EventIDs),
			"scope",
			in.scope,
			"is_test",
			in.isTest,
		)
		return &job.ID
	}

	h.logger().DebugContext(
		ctx,
		"rules enqueue: orchestrator enqueue failed; falling back",
		"error",
		err,
		"events",
		len(in.p.EventIDs),
		"scope",
		in.scope,
		"is_test",
		in.isTest,
	)
	return nil
}

func (h *EventHandlers) enqueueRulesWithJobsService(ctx context.Context, in *rulesEnqueueInput) *string {
	if in == nil || h.Jobs == nil {
		return nil
	}

	payload := service.RulesJobPayload{EventIDs: in.p.EventIDs, SiteID: in.p.SiteID, Scope: in.scope}
	b, err := json.Marshal(payload)
	if err != nil {
		h.logger().DebugContext(ctx, "rules enqueue: marshal payload failed", "error", err)
		return nil
	}

	siteIDCopy := in.p.SiteID
	req := &model.CreateJobRequest{
		Type:       model.JobTypeRules,
		Payload:    b,
		SiteID:     &siteIDCopy,
		Priority:   50,
		MaxRetries: 3,
		IsTest:     in.isTest,
	}

	j, err := h.Jobs.Create(ctx, req)
	if err != nil || j == nil {
		h.logger().DebugContext(ctx, "rules enqueue: create job failed", "error", err)
		return nil
	}

	h.logger().DebugContext(
		ctx,
		"rules enqueue: job created",
		"job_id",
		j.ID,
		"events",
		len(in.p.EventIDs),
		"scope",
		in.scope,
		"is_test",
		in.isTest,
	)
	return &j.ID
}
