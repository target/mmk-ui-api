package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/http/validation"
)

// --- Source Form (create/copy/test) ---

type sourceFormView struct {
	Data                map[string]any
	SelectedSecretNames []string
}

// renderSourceForm renders the Source create/copy form page with common framing data.
// Note: This function is domain-specific and not consolidated with other form renderers because:
// - It requires SecretOptions for the multiselect dropdown.
// - It has unique page titles and CurrentPage values.
// - Consolidation would add complexity without meaningful code reduction.
func (h *UIHandlers) renderSourceForm(w http.ResponseWriter, r *http.Request, v sourceFormView) {
	data, _ := prepareFormFrame(FormFrameOpts{
		R:           r,
		Data:        v.Data,
		DefaultMode: FormModeCreate,
		MetaForMode: func(mode FormMode) PageMeta {
			if mode == FormModeEdit {
				return PageMeta{
					Title:       "Merrymaker - Edit Source",
					PageTitle:   "Edit Source",
					CurrentPage: PageSourceForm,
				}
			}
			return PageMeta{Title: "Merrymaker - New Source", PageTitle: "New Source", CurrentPage: PageSourceForm}
		},
	})
	// Secret options for multiselect
	data["SecretOptions"] = h.buildSecretOptions(r.Context(), v.SelectedSecretNames)
	// Render
	h.renderDashboardPage(w, r, data)
}

// Helpers to parse and validate form with ≤3 params rules.

type sourceFormFields struct {
	Name        string
	Value       string
	Secrets     []string
	ClientToken string
}

type sourceFormInput struct {
	Name         string
	Value        string
	Secrets      []string
	RequireValue bool
}

func parseSourceFormFields(r *http.Request) sourceFormFields {
	return sourceFormFields{
		Name:        strings.TrimSpace(r.Form.Get("name")),
		Value:       strings.TrimSpace(r.Form.Get("value")),
		Secrets:     parseSecretsFromForm(r),
		ClientToken: strings.TrimSpace(r.Form.Get("client_token")),
	}
}

// validateSourceForm validates source form input.
// Note: Source script (value) has no max length constraint to support large Puppeteer scripts.
// The database uses TEXT type with no limit. Practical limits are enforced by HTTP request size.
func validateSourceForm(in sourceFormInput) map[string]string {
	v := validation.New().
		Validate("name", in.Name, validation.Required("Name", 255))

	if in.RequireValue {
		// Only check if value is present, no max length constraint
		value := strings.TrimSpace(in.Value)
		if value == "" {
			v.Errors()["value"] = "Source script is required."
		}
	}

	return v.Errors()
}

// SourceNew renders the create form.
func (h *UIHandlers) SourceNew(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Mode": "create"}
	h.renderSourceForm(w, r, sourceFormView{Data: data})
}

// SourceCopy renders the create form prefilled from an existing source's value and secrets.
func (h *UIHandlers) SourceCopy(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || h.SourceSvc == nil {
		h.NotFound(w, r)
		return
	}
	src, err := h.SourceSvc.GetByID(r.Context(), id)
	if err != nil || src == nil {
		h.NotFound(w, r)
		return
	}
	data := map[string]any{
		"Mode":      "create",
		"FormName":  src.Name + " Copy",
		"FormValue": src.Value,
	}
	h.renderSourceForm(w, r, sourceFormView{Data: data, SelectedSecretNames: src.Secrets})
}

// sourceFormData holds parsed form data for source creation.
type sourceFormData struct {
	Name     string
	Value    string
	Secrets  []string
	FormName string // For preserving form state
}

// parseSourceForm parses and validates source form data.
func parseSourceForm(r *http.Request) (sourceFormData, map[string]string) {
	if err := r.ParseForm(); err != nil {
		return sourceFormData{}, map[string]string{"_form": "Invalid form submission."}
	}

	fields := parseSourceFormFields(r)
	in := sourceFormInput{Name: fields.Name, Value: fields.Value, Secrets: fields.Secrets, RequireValue: true}
	errs := validateSourceForm(in)

	return sourceFormData{
		Name:     fields.Name,
		Value:    fields.Value,
		Secrets:  fields.Secrets,
		FormName: fields.Name,
	}, errs
}

// sourceFormService adapts SourcesService to work with the generic form handler.
type sourceFormService struct {
	svc SourcesService
}

func (s *sourceFormService) Create(ctx context.Context, req sourceFormData) (any, error) {
	return s.svc.Create(ctx, &model.CreateSourceRequest{
		Name:    req.Name,
		Value:   req.Value,
		Secrets: req.Secrets,
	})
}

func (s *sourceFormService) Update(_ context.Context, _ string, _ sourceFormData) (any, error) {
	// Sources don't support update operations
	return nil, errors.ErrUnsupported
}

// handleSourceError handles domain-specific errors for sources.
func handleSourceError(err error) (map[string]string, string) {
	if errors.Is(err, data.ErrSourceNameExists) {
		return map[string]string{"name": "A source with this name already exists."}, ""
	}
	// Return nil to let the default handler take over
	return nil, ""
}

// renderSourceFormWithData is a wrapper that adapts the generic form handler data to the source form renderer.
func (h *UIHandlers) renderSourceFormWithData(w http.ResponseWriter, r *http.Request, data map[string]any) {
	// Extract FormData if present and add individual fields for template compatibility
	var selectedSecrets []string
	if formData, ok := data["FormData"].(sourceFormData); ok {
		data["FormName"] = formData.FormName
		data["FormValue"] = formData.Value
		selectedSecrets = formData.Secrets
	}

	h.renderSourceForm(w, r, sourceFormView{Data: data, SelectedSecretNames: selectedSecrets})
}

// SourceCreate handles POST to create a non-test source.
func (h *UIHandlers) SourceCreate(w http.ResponseWriter, r *http.Request) {
	if h.SourceSvc == nil {
		h.NotFound(w, r)
		return
	}

	HandleForm(FormHandlerOpts[sourceFormData]{
		W:           w,
		R:           r,
		Mode:        FormModeCreate,
		Parser:      parseSourceForm,
		Service:     &sourceFormService{svc: h.SourceSvc},
		Renderer:    h.renderSourceFormWithData,
		SuccessURL:  "/sources",
		PageMeta:    PageMeta{Title: "Merrymaker - New Source", PageTitle: "New Source", CurrentPage: PageSourceForm},
		HandleError: handleSourceError,
	})
}

// validateTestAndMaybeRender validates fields for test runs and renders errors when any.
func (h *UIHandlers) validateTestAndMaybeRender(w http.ResponseWriter, r *http.Request, fields sourceFormFields) bool {
	in := sourceFormInput{Name: fields.Name, Value: fields.Value, Secrets: fields.Secrets, RequireValue: true}
	errs := validateSourceForm(in)
	delete(errs, "name") // ignore name for test
	if len(errs) > 0 {
		data := map[string]any{
			"Mode":         "create",
			"FormName":     fields.Name,
			"FormValue":    fields.Value,
			"Errors":       errs,
			"Error":        true,
			"ErrorMessage": errMsgFixBelow,
		}
		h.renderSourceForm(w, r, sourceFormView{Data: data, SelectedSecretNames: fields.Secrets})
		return false
	}
	return true
}

func buildTestRenderData(fields sourceFormFields, srcID, jobID string) map[string]any {
	return map[string]any{
		"Mode":          "create",
		"FormName":      fields.Name,
		"FormValue":     fields.Value,
		"TestRunning":   true,
		"TestSourceID":  srcID,
		"TestJobID":     jobID,
		"ClientToken":   fields.ClientToken,
		"TestStartedAt": time.Now().UTC(),
	}
}

func (h *UIHandlers) parseSourceTestFields(w http.ResponseWriter, r *http.Request) (sourceFormFields, bool) {
	if err := r.ParseForm(); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		h.renderSourceForm(w, r, sourceTestErrorView(sourceFormFields{}, "Invalid form submission."))
		return sourceFormFields{}, false
	}
	fields := parseSourceFormFields(r)
	if !h.validateTestAndMaybeRender(w, r, fields) {
		return sourceFormFields{}, false
	}
	return fields, true
}

func (h *UIHandlers) resolveTestScript(ctx context.Context, fields sourceFormFields) (string, error) {
	if len(fields.Secrets) == 0 {
		return fields.Value, nil
	}
	src := &model.Source{Value: fields.Value, Secrets: fields.Secrets}
	return h.SourceSvc.ResolveScript(ctx, src)
}

func buildTestJobRequest(script, clientToken string) (*model.CreateJobRequest, error) {
	payload := struct {
		Script string `json:"script"`
	}{Script: script}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	var meta json.RawMessage
	if clientToken != "" {
		if mb, e := json.Marshal(map[string]string{"client_token": clientToken}); e == nil {
			meta = json.RawMessage(mb)
		}
	}
	return &model.CreateJobRequest{
		Type:       model.JobTypeBrowser,
		Payload:    json.RawMessage(b),
		Metadata:   meta,
		IsTest:     true,
		MaxRetries: 0, // Test jobs should fail immediately without retrying
	}, nil
}

func sourceTestErrorView(fields sourceFormFields, message string) sourceFormView {
	return sourceFormView{
		Data: map[string]any{
			"Mode":         "create",
			"FormName":     fields.Name,
			"FormValue":    fields.Value,
			"Error":        true,
			"ErrorMessage": message,
		},
		SelectedSecretNames: fields.Secrets,
	}
}

// SourceTest handles POST to create a test source (random unique name), enqueue a browser job, and render the Test panel.
func (h *UIHandlers) SourceTest(w http.ResponseWriter, r *http.Request) {
	fields, ok := h.parseSourceTestFields(w, r)
	if !ok {
		return
	}
	if h.SourceSvc == nil {
		h.NotFound(w, r)
		return
	}

	script, err := h.resolveTestScript(r.Context(), fields)
	if err != nil {
		h.renderSourceForm(
			w,
			r,
			sourceTestErrorView(fields, "Unable to resolve secrets. Please verify selected secrets and try again."),
		)
		return
	}
	// Ephemeral test: do NOT persist a source. Enqueue a test browser job directly.
	jreq, err := buildTestJobRequest(script, fields.ClientToken)
	if err != nil {
		h.renderSourceForm(w, r, sourceTestErrorView(fields, "Unable to start test run (encode). Please try again."))
		return
	}
	job, err := h.Jobs.Create(r.Context(), jreq)
	if err != nil || job == nil {
		h.renderSourceForm(w, r, sourceTestErrorView(fields, "Unable to start test run. Please try again."))
		return
	}
	data := buildTestRenderData(fields, "", job.ID)
	h.renderSourceForm(w, r, sourceFormView{Data: data, SelectedSecretNames: fields.Secrets})
}

// SourceTestEvents renders NEW events for a source test job as an HTML fragment for HTMX polling.
// This endpoint supports incremental loading by accepting a "since" parameter (event count).
// It returns only events that occurred after the given count.
func (h *UIHandlers) SourceTestEvents(w http.ResponseWriter, r *http.Request) {
	params, ok := h.parseTestEventsParams(w, r)
	if !ok {
		return
	}

	pageEvents, ok := h.fetchTestJobEvents(w, r, testJobEventsRequest(params))
	if !ok {
		return
	}

	job, ok := h.fetchTestJobStatus(w, r, params.JobID)
	if !ok {
		return
	}

	h.renderTestEvents(w, testEventsRenderRequest{Params: params, PageEvents: pageEvents, Job: job})
}

type testEventsParams struct {
	JobID string
	Since int
}

// testJobEventsRequest groups parameters for fetchTestJobEvents (≤3 parameters rule).
type testJobEventsRequest struct {
	JobID string
	Since int
}

// testEventsRenderRequest groups parameters for renderTestEvents (≤3 parameters rule).
type testEventsRenderRequest struct {
	Params     testEventsParams
	PageEvents []*model.Event
	Job        *model.Job
}

func (h *UIHandlers) parseTestEventsParams(w http.ResponseWriter, r *http.Request) (testEventsParams, bool) {
	jobID := r.PathValue("id")
	if jobID == "" || h.EventSvc == nil || h.Jobs == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return testEventsParams{}, false
	}

	since := 0
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if parsed, err := strconv.Atoi(sinceStr); err == nil && parsed >= 0 {
			since = parsed
		}
	}

	return testEventsParams{JobID: jobID, Since: since}, true
}

func (h *UIHandlers) fetchTestJobEvents(
	w http.ResponseWriter,
	r *http.Request,
	req testJobEventsRequest,
) ([]*model.Event, bool) {
	// Use server-side pagination with since as offset
	const pageSize = 200
	opts := model.EventListByJobOptions{JobID: req.JobID, Limit: pageSize, Offset: req.Since}
	page, err := h.EventSvc.ListByJob(r.Context(), opts)
	if err != nil {
		http.Error(w, "failed to load events", http.StatusInternalServerError)
		return nil, false
	}
	return page.Events, true
}

func (h *UIHandlers) fetchTestJobStatus(w http.ResponseWriter, r *http.Request, jobID string) (*model.Job, bool) {
	job, err := h.Jobs.GetByID(r.Context(), jobID)
	if err != nil {
		http.Error(w, "failed to get job status", http.StatusInternalServerError)
		return nil, false
	}
	return job, true
}

func (h *UIHandlers) renderTestEvents(w http.ResponseWriter, req testEventsRenderRequest) {
	isTerminal := req.Job.Status == model.JobStatusCompleted || req.Job.Status == model.JobStatusFailed
	nextSince := req.Params.Since + len(req.PageEvents)

	data := map[string]any{
		"Events":     buildJobEventViews(req.Params.JobID, req.PageEvents),
		"JobID":      req.Params.JobID,
		"NextSince":  nextSince,
		"IsTerminal": isTerminal,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if templateErr := h.T.t.ExecuteTemplate(w, "source-test-events", data); templateErr != nil {
		http.Error(w, "failed to render events", http.StatusInternalServerError)
	}
}

// SourceTestStatus renders the job status for a source test as an HTML fragment for HTMX polling.
// This endpoint is used to show real-time job status updates and stops polling when the job is terminal.
func (h *UIHandlers) SourceTestStatus(w http.ResponseWriter, r *http.Request) {
	jobID := r.PathValue("id")
	if jobID == "" || h.Jobs == nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}

	job, err := h.Jobs.GetByID(r.Context(), jobID)
	if err != nil {
		http.Error(w, "failed to get job status", http.StatusInternalServerError)
		return
	}

	// Get event count for this job
	eventCount := 0
	if h.EventSvc != nil {
		// Use short timeout to avoid slow queries impacting poll requests
		ctx, cancel := context.WithTimeout(r.Context(), 200*time.Millisecond)
		defer cancel()

		opts := model.EventListByJobOptions{JobID: jobID}
		count, countErr := h.EventSvc.CountByJob(ctx, opts)
		if countErr != nil {
			// Silently ignore count errors - count will remain 0
			// This prevents slow/failing queries from breaking the status poll
			_ = countErr
		} else {
			eventCount = count
		}
	}

	data := map[string]any{
		"Job":        job,
		"EventCount": eventCount,
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if templateErr := h.T.t.ExecuteTemplate(w, "source-test-status", data); templateErr != nil {
		http.Error(w, "failed to render status", http.StatusInternalServerError)
	}
}
