package rules

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// DomainExtractor extracts a normalized domain from a raw event payload.
type DomainExtractor interface {
	ExtractDomain(event model.RawEvent) (string, bool)
}

// DomainExtractorFunc is an adapter to allow the use of ordinary functions as DomainExtractors.
type DomainExtractorFunc func(event model.RawEvent) (string, bool)

// ExtractDomain calls f(event).
func (f DomainExtractorFunc) ExtractDomain(event model.RawEvent) (string, bool) {
	if f == nil {
		return "", false
	}
	return f(event)
}

// RuleEngine evaluates rule work items and returns their individual outcomes.
type RuleEngine interface {
	Evaluate(ctx context.Context, item RuleWorkItem) []RuleEvaluation
}

// RuleEngineFunc is an adapter to allow ordinary functions to act as RuleEngines.
type RuleEngineFunc func(ctx context.Context, item RuleWorkItem) []RuleEvaluation

// Evaluate calls f(ctx, item).
func (f RuleEngineFunc) Evaluate(ctx context.Context, item RuleWorkItem) []RuleEvaluation {
	if f == nil {
		return nil
	}
	return f(ctx, item)
}

// RuleWorkItem captures the context required to evaluate a single event.
type RuleWorkItem struct {
	Event      *model.Event
	SiteID     string
	Scope      string
	Domain     string
	DryRun     bool
	AlertMode  model.SiteAlertMode
	JobID      string
	EventID    string
	RequestURL string
	PageURL    string
	Referrer   string
	UserAgent  string
}

// RuleEvaluation captures the result of invoking a single rule.
type RuleEvaluation struct {
	RuleID  string
	ApplyFn func(*ProcessingResults)
	Err     error
}

// Apply applies the evaluation's mutation to the provided aggregate results, when present.
func (e RuleEvaluation) Apply(results *ProcessingResults) {
	if e.ApplyFn == nil || results == nil {
		return
	}
	e.ApplyFn(results)
}

// PipelineOptions configure the behavior of the default pipeline implementation.
type PipelineOptions struct {
	Engine    RuleEngine
	Extractor DomainExtractor
	Logger    *slog.Logger
}

// DefaultPipeline executes the rules evaluation workflow for a batch of events.
type DefaultPipeline struct {
	engine    RuleEngine
	extractor DomainExtractor
	logger    *slog.Logger
}

// NewPipeline constructs a DefaultPipeline from the supplied options.
func NewPipeline(opts PipelineOptions) *DefaultPipeline {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	return &DefaultPipeline{
		engine:    opts.Engine,
		extractor: opts.Extractor,
		logger:    logger,
	}
}

// Run executes the rules evaluation workflow for the provided params.
func (p *DefaultPipeline) Run(
	ctx context.Context,
	params PipelineParams,
) (*ProcessingResults, error) {
	payload := params.Payload
	alertMode := params.AlertMode
	alertMode = normalizeAlertMode(alertMode)

	results := &ProcessingResults{
		IsDryRun:  params.DryRun,
		AlertMode: alertMode,
	}

	if len(params.Events) == 0 {
		return results, nil
	}

	start := time.Now()

	if err := ctx.Err(); err != nil {
		results.ProcessingTime = time.Since(start)
		return results, err
	}

	if p.engine == nil {
		results.ProcessingTime = time.Since(start)
		return results, nil
	}

	siteID := ""
	scope := ""
	if payload != nil {
		siteID = payload.SiteID
		scope = payload.Scope
	}

	baseItem := RuleWorkItem{
		SiteID:    siteID,
		Scope:     scope,
		DryRun:    params.DryRun,
		AlertMode: alertMode,
		JobID:     params.JobID,
	}

	for _, event := range params.Events {
		if err := ctx.Err(); err != nil {
			results.ProcessingTime = time.Since(start)
			return results, err
		}

		item := baseItem
		item.Event = event

		p.processEvent(ctx, results, item)
	}
	results.ProcessingTime = time.Since(start)

	return results, nil
}

func (p *DefaultPipeline) processEvent(
	ctx context.Context,
	results *ProcessingResults,
	item RuleWorkItem,
) {
	if item.Event == nil {
		results.EventsSkipped++
		return
	}

	item.EventID = item.Event.ID
	reqCtx := extractRequestContext(item.Event)
	item.RequestURL = reqCtx.RequestURL
	item.PageURL = reqCtx.PageURL
	item.Referrer = reqCtx.Referrer
	item.UserAgent = reqCtx.UserAgent

	domain, ok := p.extractDomain(item.Event)
	if !ok {
		results.EventsSkipped++
		return
	}

	results.DomainsProcessed++

	item.Domain = domain

	p.applyEvaluations(ctx, results, item)
}

func (p *DefaultPipeline) extractDomain(event *model.Event) (string, bool) {
	if event == nil || p.extractor == nil {
		return "", false
	}
	return p.extractor.ExtractDomain(model.RawEvent{
		Type: event.EventType,
		Data: event.EventData,
	})
}

func (p *DefaultPipeline) applyEvaluations(
	ctx context.Context,
	results *ProcessingResults,
	item RuleWorkItem,
) {
	if p.engine == nil {
		return
	}
	evaluations := p.engine.Evaluate(ctx, item)
	for _, eval := range evaluations {
		if eval.Err != nil {
			p.logger.ErrorContext(ctx, "rule evaluation failed",
				"rule_id", eval.RuleID,
				"domain", item.Domain,
				"site_id", item.SiteID,
				"scope", item.Scope,
				"err", eval.Err)
			results.ErrorsEncountered++
			continue
		}
		eval.Apply(results)
	}
}

var _ Pipeline = (*DefaultPipeline)(nil)

type requestContext struct {
	RequestURL string
	PageURL    string
	Referrer   string
	UserAgent  string
}

func extractRequestContext(evt *model.Event) requestContext {
	ctx := requestContext{}
	if evt == nil {
		return ctx
	}

	ctx.RequestURL = extractRequestURL(evt.EventType, evt.EventData)
	ctx.Referrer = extractReferrer(evt.EventData)
	if attr := extractAttribution(evt.Metadata); attr != nil {
		ctx.PageURL = strings.TrimSpace(attr.URL)
		ctx.UserAgent = strings.TrimSpace(attr.UserAgent)
	}

	return ctx
}

func extractRequestURL(eventType string, data json.RawMessage) string {
	if !strings.HasPrefix(eventType, "Network.") || len(data) == 0 {
		return ""
	}

	type reqShape struct {
		Request struct {
			URL     string         `json:"url"`
			Headers map[string]any `json:"headers"`
		} `json:"request"`
		URL      string `json:"url"`
		Response struct {
			URL string `json:"url"`
		} `json:"response"`
	}

	var p reqShape
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}

	candidates := []string{
		strings.TrimSpace(p.Request.URL),
		strings.TrimSpace(p.URL),
		strings.TrimSpace(p.Response.URL),
	}
	for _, c := range candidates {
		if c != "" {
			return c
		}
	}
	return ""
}

func extractReferrer(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}

	type reqShape struct {
		Request struct {
			Headers map[string]any `json:"headers"`
		} `json:"request"`
	}

	var p reqShape
	if err := json.Unmarshal(data, &p); err != nil {
		return ""
	}

	// Prefer standard HTTP header spelling, but also handle "Referrer" producers.
	ref := findHeader(p.Request.Headers, "referer")
	if ref != "" {
		return ref
	}
	return findHeader(p.Request.Headers, "referrer")
}

func findHeader(headers map[string]any, name string) string {
	if len(headers) == 0 {
		return ""
	}
	lower := strings.ToLower(name)
	for k, v := range headers {
		if strings.ToLower(k) != lower {
			continue
		}
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func extractAttribution(meta json.RawMessage) *model.PuppeteerAttribution {
	if len(meta) == 0 {
		return nil
	}
	var parsed struct {
		Attribution *model.PuppeteerAttribution `json:"attribution"`
	}
	if err := json.Unmarshal(meta, &parsed); err != nil {
		return nil
	}
	return parsed.Attribution
}
