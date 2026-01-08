package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/http/validation"
)

// sampleAlertEventContext represents the superset of fields that may appear in event_context
// across different rule types (unknown_domain, ioc_domain, yara_rule, custom).
type sampleAlertEventContext struct {
	Domain     string `json:"domain,omitempty"`
	Host       string `json:"host,omitempty"`
	Scope      string `json:"scope"`
	SiteID     string `json:"site_id"`
	JobID      string `json:"job_id,omitempty"`
	EventID    string `json:"event_id,omitempty"`
	RequestURL string `json:"request_url,omitempty"`
	PageURL    string `json:"page_url,omitempty"`
	Referrer   string `json:"referrer,omitempty"`
	UserAgent  string `json:"user_agent,omitempty"`
	IOCID      string `json:"ioc_id,omitempty"`
	IOCType    string `json:"ioc_type,omitempty"`
	IOCValue   string `json:"ioc_value,omitempty"`
}

// sampleAlertPayload mirrors model.Alert structure for generating sample JSON.
// This ensures the sample stays in sync with the actual payload sent to sinks.
type sampleAlertPayload struct {
	ID             string                    `json:"id"`
	SiteID         string                    `json:"site_id"`
	RuleID         *string                   `json:"rule_id"`
	RuleType       string                    `json:"rule_type"`
	Severity       string                    `json:"severity"`
	Title          string                    `json:"title"`
	Description    string                    `json:"description"`
	EventContext   sampleAlertEventContext   `json:"event_context"`
	Metadata       map[string]any            `json:"metadata"`
	DeliveryStatus model.AlertDeliveryStatus `json:"delivery_status"`
	FiredAt        time.Time                 `json:"fired_at"`
	ResolvedAt     *time.Time                `json:"resolved_at"`
	ResolvedBy     *string                   `json:"resolved_by"`
	CreatedAt      time.Time                 `json:"created_at"`
}

// buildSampleAlertJSON generates a representative sample alert payload for the JMESPath preview.
// It includes fields from multiple rule types so users can build transformations that work across all alerts.
func buildSampleAlertJSON() string {
	sampleTime := time.Date(2021, 6, 1, 0, 0, 0, 0, time.UTC)
	sample := sampleAlertPayload{
		ID:          "alert-8a497372-6b43-426c-a323-37706302589c",
		SiteID:      "site-001",
		RuleID:      nil,
		RuleType:    string(model.AlertRuleTypeUnknownDomain),
		Severity:    string(model.AlertSeverityMedium),
		Title:       "Unknown domain observed",
		Description: "First time seen domain: example.com (scope: production)",
		EventContext: sampleAlertEventContext{
			Domain:     "example.com",
			Host:       "example.com",
			Scope:      "production",
			SiteID:     "site-001",
			JobID:      "job-abc123",
			EventID:    "evt-xyz789",
			RequestURL: "https://example.com/resource.js",
			PageURL:    "https://mysite.com/page",
			Referrer:   "https://google.com",
			UserAgent:  "Mozilla/5.0",
			IOCID:      "ioc-001",
			IOCType:    "domain",
			IOCValue:   "example.com",
		},
		Metadata:       map[string]any{},
		DeliveryStatus: model.AlertDeliveryStatusPending,
		FiredAt:        sampleTime,
		ResolvedAt:     nil,
		ResolvedBy:     nil,
		CreatedAt:      sampleTime,
	}
	b, err := json.MarshalIndent(sample, "", "  ")
	if err != nil {
		return "{}"
	}
	return string(b)
}

// buildSecretOptions returns a slice of {Name, Selected} maps for the secrets select.
func (h *UIHandlers) buildSecretOptions(ctx context.Context, selected []string) []map[string]any {
	var out []map[string]any
	if h.SecretSvc == nil {
		return out
	}
	// Fetch up to 1000 for options; adjust if needed later
	list, err := h.SecretSvc.List(ctx, 1000, 0)
	if err != nil {
		return out
	}
	// Sort by name (case-insensitive) for stable UX ordering
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	set := map[string]struct{}{}
	for _, s := range selected {
		set[strings.TrimSpace(s)] = struct{}{}
	}
	out = make([]map[string]any, 0, len(list))
	for _, s := range list {
		_, sel := set[s.Name]
		out = append(out, map[string]any{"Name": s.Name, "Selected": sel})
	}
	return out
}

// parseSecretsFromForm collects non-empty unique secret names from a POSTed form.
func parseSecretsFromForm(r *http.Request) []string {
	m := map[string]struct{}{}
	var out []string
	for _, v := range r.Form["secrets"] {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if _, ok := m[v]; ok {
			continue
		}
		m[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// alertSinkFormInput groups inputs to keep parameter count small.
type alertSinkFormInput struct {
	Name, Method, URI, OkStatusStr, RetryStr string
	Secrets                                  []string
	RequireSecrets                           bool
}

// parseOptionalHTTPStatus parses an optional HTTP status code.
func parseOptionalHTTPStatus(s string) (*int, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ""
	}
	v, e := strconv.Atoi(s)
	if e != nil || v < 100 || v > 599 {
		return nil, "OK status must be 100-599."
	}
	return &v, ""
}

// parseOptionalRetry parses an optional retry count.
func parseOptionalRetry(s string) (*int, string) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, ""
	}
	v, e := strconv.Atoi(s)
	if e != nil || v < 0 {
		return nil, "Retry must be 0 or greater."
	}
	return &v, ""
}

// validateAlertSinkForm validates user input and returns an error map and parsed numeric pointers.
func validateAlertSinkForm(in alertSinkFormInput) (map[string]string, *int, *int) {
	v := validation.New().
		Validate("name", in.Name, validation.RequiredRange("Name", 3, 512)).
		Validate("method", in.Method, validation.OneOf("Method", []string{"GET", "POST", "PUT", "DELETE"})).
		Validate("uri", in.URI, validation.HTTPSURL("URI", 1024))

	// Custom validation for secrets requirement
	if in.RequireSecrets && len(in.Secrets) == 0 {
		v.Errors()["secrets"] = "Select at least one secret."
	}

	// Parse and validate optional numeric fields
	okPtr, msg := parseOptionalHTTPStatus(in.OkStatusStr)
	if msg != "" {
		v.Errors()["ok_status"] = msg
	}
	retryPtr, msg := parseOptionalRetry(in.RetryStr)
	if msg != "" {
		v.Errors()["retry"] = msg
	}

	return v.Errors(), okPtr, retryPtr
}

func alertSinkFormMeta(mode FormMode) PageMeta {
	if mode == FormModeEdit {
		return PageMeta{
			Title:       "Merrymaker - Edit HTTP Alert Sink",
			PageTitle:   "Edit HTTP Alert Sink",
			CurrentPage: PageAlertSinkForm,
		}
	}
	return PageMeta{
		Title:       "Merrymaker - New HTTP Alert Sink",
		PageTitle:   "New HTTP Alert Sink",
		CurrentPage: PageAlertSinkForm,
	}
}

func resolveAlertSinkFormMode(data map[string]any) FormMode {
	if data == nil {
		return FormModeCreate
	}
	if mode, ok := data["Mode"].(FormMode); ok && mode != "" {
		return mode
	}
	if modeStr, ok := data["Mode"].(string); ok && modeStr != "" {
		return FormMode(modeStr)
	}
	return FormModeCreate
}

// renderAlertSinkFormWithData adapts generic form handler data to the alert sink templates.
func (h *UIHandlers) renderAlertSinkFormWithData(
	w http.ResponseWriter,
	r *http.Request,
	data map[string]any,
) {
	mode := resolveAlertSinkFormMode(data)
	data, mode = prepareFormFrame(FormFrameOpts{
		R:           r,
		Data:        data,
		DefaultMode: mode,
	})
	if _, ok := data["FormData"]; !ok {
		data["FormData"] = alertSinkFormData{}
	}

	h.prepareAlertSinkFormData(r, mode, data)

	builder := NewTemplateData(r, alertSinkFormMeta(mode))
	for k, v := range data {
		builder.With(k, v)
	}

	h.renderDashboardPage(w, r, builder.Build())
}

// AlertSinkNew renders the create form.
func (h *UIHandlers) AlertSinkNew(w http.ResponseWriter, r *http.Request) {
	h.renderAlertSinkFormWithData(w, r, map[string]any{
		"Mode":     FormModeCreate,
		"FormData": alertSinkFormData{},
	})
}

// AlertSinkEdit renders the edit form for a given sink id.
func (h *UIHandlers) AlertSinkEdit(w http.ResponseWriter, r *http.Request) {
	if h.Sinks == nil {
		h.NotFound(w, r)
		return
	}
	id := r.PathValue("id")
	if id == "" {
		h.NotFound(w, r)
		return
	}
	sink, err := h.Sinks.GetByID(r.Context(), id)
	if err != nil || sink == nil {
		h.NotFound(w, r)
		return
	}
	h.renderAlertSinkFormWithData(w, r, map[string]any{
		"Mode":      FormModeEdit,
		"AlertSink": buildAlertSinkTemplateData(sink),
	})
}

// alertSinkFormData holds parsed form data for alert sink creation and updates.
type alertSinkFormData struct {
	Name        string
	Method      string
	URI         string
	QueryParams string
	Headers     string
	Body        string
	OkStatus    *int
	Retry       *int
	Secrets     []string
	// Form state preservation
	FormName        string
	FormMethod      string
	FormURI         string
	FormQueryParams string
	FormHeaders     string
	FormBody        string
	FormOkStatus    string
	FormRetry       string
}

// canonicalMethod normalizes HTTP method strings.
func canonicalMethod(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// parseAlertSinkForm parses and validates alert sink form data.
func parseAlertSinkForm(r *http.Request) (alertSinkFormData, map[string]string) {
	if err := r.ParseForm(); err != nil {
		return alertSinkFormData{}, map[string]string{"_form": "Invalid form submission."}
	}

	fields, in := bindAlertSinkInput(r, false)
	errs, okPtr, retryPtr := validateAlertSinkForm(in)

	return alertSinkFormData{
		Name:            fields.Name,
		Method:          fields.Method,
		URI:             fields.URI,
		QueryParams:     fields.QueryParams,
		Headers:         fields.Headers,
		Body:            fields.Body,
		OkStatus:        okPtr,
		Retry:           retryPtr,
		Secrets:         fields.Secrets,
		FormName:        fields.Name,
		FormMethod:      canonicalMethod(fields.Method),
		FormURI:         fields.URI,
		FormQueryParams: fields.QueryParams,
		FormHeaders:     fields.Headers,
		FormBody:        fields.Body,
		FormOkStatus:    strings.TrimSpace(fields.OkStatusStr),
		FormRetry:       strings.TrimSpace(fields.RetryStr),
	}, errs
}

// alertSinkFormService adapts AlertSinksService to work with the generic form handler.
type alertSinkFormService struct {
	svc AlertSinksService
}

func (s *alertSinkFormService) Create(ctx context.Context, req alertSinkFormData) (any, error) {
	var qpPtr, hPtr, bPtr *string
	if req.QueryParams != "" {
		qpPtr = &req.QueryParams
	}
	if req.Headers != "" {
		hPtr = &req.Headers
	}
	if req.Body != "" {
		bPtr = &req.Body
	}

	return s.svc.Create(ctx, &model.CreateHTTPAlertSinkRequest{
		Name:        req.Name,
		URI:         req.URI,
		Method:      canonicalMethod(req.Method),
		QueryParams: qpPtr,
		Headers:     hPtr,
		Body:        bPtr,
		OkStatus:    req.OkStatus,
		Retry:       req.Retry,
		Secrets:     req.Secrets,
	})
}

func (s *alertSinkFormService) Update(ctx context.Context, id string, req alertSinkFormData) (any, error) {
	qpPtr := &req.QueryParams
	hPtr := &req.Headers
	bPtr := &req.Body
	nameTrim := strings.TrimSpace(req.Name)
	methodCanon := canonicalMethod(req.Method)

	return s.svc.Update(ctx, id, &model.UpdateHTTPAlertSinkRequest{
		Name:        &nameTrim,
		Method:      &methodCanon,
		URI:         &req.URI,
		QueryParams: qpPtr,
		Headers:     hPtr,
		Body:        bPtr,
		OkStatus:    req.OkStatus,
		Retry:       req.Retry,
		Secrets:     req.Secrets,
	})
}

// handleAlertSinkError handles domain-specific errors for alert sinks.
func handleAlertSinkFormError(err error) (map[string]string, string) {
	if errors.Is(err, data.ErrHTTPAlertSinkNotFound) {
		return nil, "Alert sink not found."
	}
	if errors.Is(err, data.ErrHTTPAlertSinkNameExists) {
		return map[string]string{"name": "A sink with this name already exists."}, ""
	}
	// Return nil to let the default handler take over
	return nil, ""
}

func normalizeAlertSinkTemplate(existing any) (map[string]any, bool) {
	switch sink := existing.(type) {
	case map[string]any:
		return sink, true
	case *model.HTTPAlertSink:
		return buildAlertSinkTemplateData(sink), true
	case model.HTTPAlertSink:
		s := sink
		return buildAlertSinkTemplateData(&s), true
	default:
		return nil, false
	}
}

// loadAlertSinkForEdit loads the alert sink for edit mode if not already present in templateData.
func (h *UIHandlers) loadAlertSinkForEdit(r *http.Request, templateData map[string]any) {
	if existing, hasAlertSink := templateData["AlertSink"]; hasAlertSink {
		if normalized, ok := normalizeAlertSinkTemplate(existing); ok {
			templateData["AlertSink"] = normalized
			return
		}
	}
	id := r.PathValue("id")
	if id == "" || h.Sinks == nil {
		return
	}
	sink, err := h.Sinks.GetByID(r.Context(), id)
	if err == nil && sink != nil {
		templateData["AlertSink"] = buildAlertSinkTemplateData(sink)
		return
	}

	// Distinguish between not-found and internal errors
	if err != nil && !errors.Is(err, data.ErrHTTPAlertSinkNotFound) {
		// Internal error - surface it to the user
		templateData["Error"] = true
		templateData["ErrorMessage"] = "Unable to load alert sink. Please try again."
	}

	// Use a placeholder with the ID to prevent template errors
	templateData["AlertSink"] = map[string]any{"ID": id}
}

func (h *UIHandlers) prepareAlertSinkFormData(r *http.Request, mode FormMode, data map[string]any) {
	selected := extractAlertSinkSecrets(data)

	if mode == FormModeEdit {
		h.loadAlertSinkForEdit(r, data)
		if len(selected) == 0 {
			selected = secretsFromTemplate(data)
		}
	}

	data["SecretOptions"] = h.buildSecretOptions(r.Context(), selected)
	data["SampleAlertJSON"] = buildSampleAlertJSON()
}

func extractAlertSinkSecrets(data map[string]any) []string {
	formData, ok := data["FormData"].(alertSinkFormData)
	if !ok {
		return nil
	}

	data["FormName"] = formData.FormName
	data["FormMethod"] = formData.FormMethod
	data["FormURI"] = formData.FormURI
	data["FormQueryParams"] = formData.FormQueryParams
	data["FormHeaders"] = formData.FormHeaders
	data["FormBody"] = formData.FormBody
	if formData.FormOkStatus != "" {
		data["FormOkStatus"] = formData.FormOkStatus
	}
	if formData.FormRetry != "" {
		data["FormRetry"] = formData.FormRetry
	}

	if len(formData.Secrets) == 0 {
		return nil
	}

	out := make([]string, len(formData.Secrets))
	copy(out, formData.Secrets)
	return out
}

func secretsFromTemplate(data map[string]any) []string {
	sink, ok := data["AlertSink"].(map[string]any)
	if !ok {
		return nil
	}
	raw, ok := sink["Secrets"].([]string)
	if !ok || len(raw) == 0 {
		return nil
	}

	out := make([]string, len(raw))
	copy(out, raw)
	return out
}

// buildAlertSinkTemplateData converts an HTTPAlertSink to template data, safely dereferencing pointer fields.
func buildAlertSinkTemplateData(sink *model.HTTPAlertSink) map[string]any {
	var headers, queryParams, body string
	if sink.Headers != nil {
		headers = *sink.Headers
	}
	if sink.QueryParams != nil {
		queryParams = *sink.QueryParams
	}
	if sink.Body != nil {
		body = *sink.Body
	}

	data := map[string]any{
		"ID": sink.ID, "Name": sink.Name, "URI": sink.URI, "Method": sink.Method,
		"Headers": headers, "QueryParams": queryParams, "Body": body,
		"OkStatus": sink.OkStatus, "Retry": sink.Retry,
	}

	if len(sink.Secrets) > 0 {
		clone := make([]string, len(sink.Secrets))
		copy(clone, sink.Secrets)
		data["Secrets"] = clone
	}

	return data
}

// AlertSinkCreate handles POST from the create form.
func (h *UIHandlers) AlertSinkCreate(w http.ResponseWriter, r *http.Request) {
	if h.Sinks == nil {
		h.NotFound(w, r)
		return
	}

	HandleForm(FormHandlerOpts[alertSinkFormData]{
		W:           w,
		R:           r,
		Mode:        FormModeCreate,
		Parser:      parseAlertSinkForm,
		Service:     &alertSinkFormService{svc: h.Sinks},
		Renderer:    h.renderAlertSinkFormWithData,
		SuccessURL:  alertSinksBasePath,
		PageMeta:    alertSinkFormMeta(FormModeCreate),
		HandleError: handleAlertSinkFormError,
	})
}

// helpers for AlertSinkUpdate to reduce function length.
type alertSinkFormFields struct {
	Name, Method, URI, QueryParams, Headers, Body string
	Secrets                                       []string
	OkStatusStr, RetryStr                         string
}

func parseAlertSinkFormFields(r *http.Request) alertSinkFormFields {
	return alertSinkFormFields{
		Name:        r.Form.Get("name"),
		Method:      r.Form.Get("method"),
		URI:         r.Form.Get("uri"),
		Secrets:     parseSecretsFromForm(r),
		QueryParams: strings.TrimSpace(r.Form.Get("query_params")),
		Headers:     strings.TrimSpace(r.Form.Get("headers")),
		Body:        strings.TrimSpace(r.Form.Get("body")),
		OkStatusStr: r.Form.Get("ok_status"),
		RetryStr:    r.Form.Get("retry"),
	}
}

// bindAlertSinkInput parses form fields and prepares the validation input.
func bindAlertSinkInput(r *http.Request, requireSecrets bool) (alertSinkFormFields, alertSinkFormInput) {
	f := parseAlertSinkFormFields(r)
	in := alertSinkFormInput{
		Name: f.Name, Method: f.Method, URI: f.URI,
		OkStatusStr: f.OkStatusStr, RetryStr: f.RetryStr,
		Secrets: f.Secrets, RequireSecrets: requireSecrets,
	}
	return f, in
}

// getAlertSinkID validates that the alert sink exists before processing the form.
func (h *UIHandlers) getAlertSinkID(r *http.Request) string {
	id := r.PathValue("id")
	if id == "" || h.Sinks == nil {
		return ""
	}
	// Check if the alert sink exists
	sink, err := h.Sinks.GetByID(r.Context(), id)
	if err != nil || sink == nil {
		return ""
	}
	return id
}

// AlertSinkUpdate handles POST from the edit form.
func (h *UIHandlers) AlertSinkUpdate(w http.ResponseWriter, r *http.Request) {
	if h.Sinks == nil {
		h.NotFound(w, r)
		return
	}

	HandleForm(FormHandlerOpts[alertSinkFormData]{
		W:           w,
		R:           r,
		Mode:        FormModeEdit,
		Parser:      parseAlertSinkForm,
		Service:     &alertSinkFormService{svc: h.Sinks},
		Renderer:    h.renderAlertSinkFormWithData,
		SuccessURL:  alertSinksBasePath,
		PageMeta:    alertSinkFormMeta(FormModeEdit),
		GetID:       h.getAlertSinkID,
		HandleError: handleAlertSinkFormError,
	})
}
