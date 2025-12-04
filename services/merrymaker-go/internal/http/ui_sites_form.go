package httpx

import (
	"context"
	"errors"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/http/validation"
)

// --- Site Form (create/edit/test) ---

const optionListLimit = 10000 // named to avoid magic numbers; adjust as needed

// buildSourceOptions returns [{ID, Name, Selected}] for the source select.
func (h *UIHandlers) buildSourceOptions(ctx context.Context, selectedID string) ([]map[string]any, error) {
	var out []map[string]any
	if h.SourceSvc == nil {
		return out, errors.New("sources service unavailable")
	}
	list, err := h.SourceSvc.List(ctx, optionListLimit, 0)
	if err != nil {
		return out, err
	}
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	for _, s := range list {
		out = append(out, map[string]any{
			"ID":       s.ID,
			"Name":     s.Name,
			"Selected": s.ID == selectedID,
		})
	}
	return out, nil
}

// buildAlertSinkOptions returns [{ID, Name, Selected}] for the alert sink select.
func (h *UIHandlers) buildAlertSinkOptions(ctx context.Context, selectedID string) ([]map[string]any, error) {
	var out []map[string]any
	if h.Sinks == nil {
		return out, errors.New("alert sinks service unavailable")
	}
	list, err := h.Sinks.List(ctx, optionListLimit, 0)
	if err != nil {
		return out, err
	}
	sort.Slice(list, func(i, j int) bool { return strings.ToLower(list[i].Name) < strings.ToLower(list[j].Name) })
	for _, s := range list {
		out = append(out, map[string]any{
			"ID":       s.ID,
			"Name":     s.Name,
			"Selected": s.ID == selectedID,
		})
	}
	return out, nil
}

// renderSiteForm renders the Site create/edit form page with common framing data.
// Note: This function is domain-specific and not consolidated with other form renderers because:
// - It requires Source and AlertSink options for dropdown selects.
// - It has unique page titles and CurrentPage values.
// - Consolidation would add complexity without meaningful code reduction.
func (h *UIHandlers) renderSiteForm(w http.ResponseWriter, r *http.Request, data map[string]any) {
	data = h.prepareSiteFormFrame(r, data)
	h.loadSiteFormOptions(r.Context(), data)
	h.renderDashboardPage(w, r, data)
}

func toString(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

func titlesForMode(mode FormMode) (string, string) {
	if mode == FormModeEdit {
		return "Merrymaker - Edit Site", "Edit Site"
	}
	return "Merrymaker - New Site", "New Site"
}

func (h *UIHandlers) prepareSiteFormFrame(r *http.Request, data map[string]any) map[string]any {
	data, _ = prepareFormFrame(FormFrameOpts{
		R:           r,
		Data:        data,
		DefaultMode: FormModeCreate,
		MetaForMode: func(mode FormMode) PageMeta {
			title, pageTitle := titlesForMode(mode)
			return PageMeta{Title: title, PageTitle: pageTitle, CurrentPage: PageSiteForm}
		},
	})
	return data
}

func composeOptionsError(srcErr, sinkErr error) (bool, string) {
	if srcErr == nil && sinkErr == nil {
		return false, ""
	}
	if srcErr != nil && sinkErr != nil {
		return true, "Failed to load Sources and Alert Sinks."
	}
	if srcErr != nil {
		return true, "Failed to load Sources."
	}
	return true, "Failed to load Alert Sinks."
}

func (h *UIHandlers) loadSiteFormOptions(ctx context.Context, data map[string]any) {
	srcOpts, srcErr := h.buildSourceOptions(ctx, toString(data["FormSourceID"]))
	sinkOpts, sinkErr := h.buildAlertSinkOptions(ctx, toString(data["FormAlertSinkID"]))
	data["SourceOptions"] = srcOpts
	data["AlertSinkOptions"] = sinkOpts
	if hasErr, msg := composeOptionsError(srcErr, sinkErr); hasErr {
		data["Error"], data["ErrorMessage"] = true, msg
	}
}

type siteFormFields struct {
	Name            string
	Enabled         bool
	AlertMode       string
	Scope           string
	AlertSinkID     string
	RunEveryMinutes int
	SourceID        string
}

func parseSiteFormFields(r *http.Request) (siteFormFields, map[string]string) {
	errs := map[string]string{}
	if err := r.ParseForm(); err != nil {
		errs["_"] = "Invalid form submission."
	}
	enabled := r.Form.Get("enabled") == "on"
	mode := strings.TrimSpace(strings.ToLower(r.Form.Get("alert_mode")))
	if mode == "" {
		mode = string(model.SiteAlertModeActive)
	}
	runTxt := strings.TrimSpace(r.Form.Get("run_every_minutes"))
	runMins := 0
	if runTxt != "" {
		n, err := strconv.Atoi(runTxt)
		if err != nil {
			errs["run_every_minutes"] = "Invalid number."
		} else {
			runMins = n
		}
	}
	fields := siteFormFields{
		Name:            strings.TrimSpace(r.Form.Get("name")),
		Enabled:         enabled,
		AlertMode:       mode,
		Scope:           strings.TrimSpace(r.Form.Get("scope")),
		AlertSinkID:     strings.TrimSpace(r.Form.Get("alert_sink_id")),
		RunEveryMinutes: runMins,
		SourceID:        strings.TrimSpace(r.Form.Get("source_id")),
	}
	return fields, errs
}

func validateSiteForm(f siteFormFields) map[string]string {
	v := validation.New().
		Validate("name", f.Name, validation.Required("Name", 255)).
		Validate("source_id", f.SourceID, validation.Required("Source", 255)).
		Validate("alert_sink_id", f.AlertSinkID, validation.Required("Alert Sink", 255))

	// Custom validation for run_every_minutes (must be > 0)
	if f.RunEveryMinutes <= 0 {
		v.Errors()["run_every_minutes"] = "Run interval must be > 0."
	}

	mode := model.SiteAlertMode(f.AlertMode)
	if !mode.Valid() {
		v.Errors()["alert_mode"] = "Select a valid alert mode."
	}

	return v.Errors()
}

// --- Handlers ---

// SiteNew renders the create form.
func (h *UIHandlers) SiteNew(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Mode":                "create",
		"FormRunEveryMinutes": 15,
		"FormAlertMode":       string(model.SiteAlertModeActive),
	}
	h.renderSiteForm(w, r, data)
}

// SiteEdit renders the edit form populated from an existing site.
func (h *UIHandlers) SiteEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || h.SiteSvc == nil {
		h.NotFound(w, r)
		return
	}
	s, err := h.SiteSvc.GetByID(r.Context(), id)
	if err != nil || s == nil {
		h.NotFound(w, r)
		return
	}
	var scope, sinkID string
	if s.Scope != nil {
		scope = strings.TrimSpace(*s.Scope)
	}
	if s.HTTPAlertSinkID != nil {
		sinkID = strings.TrimSpace(*s.HTTPAlertSinkID)
	}
	data := map[string]any{
		"Mode":                FormModeEdit,
		"SiteID":              s.ID,
		"FormName":            s.Name,
		"FormEnabled":         s.Enabled,
		"FormAlertMode":       string(s.AlertMode),
		"FormScope":           scope,
		"FormAlertSinkID":     sinkID,
		"FormRunEveryMinutes": s.RunEveryMinutes,
		"FormSourceID":        s.SourceID,
	}
	h.renderSiteForm(w, r, data)
}

// siteFormData holds parsed form data for site creation and updates.
type siteFormData struct {
	Name            string
	Enabled         bool
	AlertMode       string
	Scope           string
	AlertSinkID     string
	RunEveryMinutes int
	SourceID        string
	// Form state preservation
	FormName            string
	FormEnabled         bool
	FormAlertMode       string
	FormScope           string
	FormAlertSinkID     string
	FormRunEveryMinutes int
	FormSourceID        string
}

// parseSiteForm parses and validates site form data.
func parseSiteForm(r *http.Request) (siteFormData, map[string]string) {
	fields, parseErrs := parseSiteFormFields(r)
	errs := validateSiteForm(fields)
	for k, v := range parseErrs {
		if v != "" {
			errs[k] = v
		}
	}

	return siteFormData{
		Name:                fields.Name,
		Enabled:             fields.Enabled,
		AlertMode:           fields.AlertMode,
		Scope:               fields.Scope,
		AlertSinkID:         fields.AlertSinkID,
		RunEveryMinutes:     fields.RunEveryMinutes,
		SourceID:            fields.SourceID,
		FormName:            fields.Name,
		FormEnabled:         fields.Enabled,
		FormAlertMode:       fields.AlertMode,
		FormScope:           fields.Scope,
		FormAlertSinkID:     fields.AlertSinkID,
		FormRunEveryMinutes: fields.RunEveryMinutes,
		FormSourceID:        fields.SourceID,
	}, errs
}

// siteFormService adapts SitesService to work with the generic form handler.
type siteFormService struct {
	svc SitesService
}

func (s *siteFormService) Create(ctx context.Context, req siteFormData) (any, error) {
	var scopePtr *string
	if req.Scope != "" {
		scopePtr = &req.Scope
	}
	sinkID := req.AlertSinkID
	mode := model.SiteAlertMode(req.AlertMode)
	return s.svc.Create(ctx, &model.CreateSiteRequest{
		Name:            req.Name,
		Enabled:         &req.Enabled,
		Scope:           scopePtr,
		HTTPAlertSinkID: &sinkID,
		RunEveryMinutes: req.RunEveryMinutes,
		SourceID:        req.SourceID,
		AlertMode:       mode,
	})
}

func (s *siteFormService) Update(ctx context.Context, id string, req siteFormData) (any, error) {
	var updateReq model.UpdateSiteRequest
	name := req.Name
	updateReq.Name = &name
	updateReq.Enabled = &req.Enabled
	if req.AlertMode != "" {
		mode := model.SiteAlertMode(req.AlertMode)
		updateReq.AlertMode = &mode
	}
	if req.Scope == "" {
		s := ""
		updateReq.Scope = &s
	} else {
		updateReq.Scope = &req.Scope
	}
	if req.AlertSinkID != "" {
		updateReq.HTTPAlertSinkID = &req.AlertSinkID
	}
	run := req.RunEveryMinutes
	updateReq.RunEveryMinutes = &run
	updateReq.SourceID = &req.SourceID
	return s.svc.Update(ctx, id, updateReq)
}

// renderSiteFormWithData is a wrapper that adapts the generic form handler data to the site form renderer.
func (h *UIHandlers) renderSiteFormWithData(w http.ResponseWriter, r *http.Request, data map[string]any) {
	// Extract FormData if present and add individual fields for template compatibility
	if formData, ok := data["FormData"].(siteFormData); ok {
		data["FormName"] = formData.FormName
		data["FormEnabled"] = formData.FormEnabled
		data["FormAlertMode"] = formData.FormAlertMode
		data["FormScope"] = formData.FormScope
		data["FormAlertSinkID"] = formData.FormAlertSinkID
		data["FormRunEveryMinutes"] = formData.FormRunEveryMinutes
		data["FormSourceID"] = formData.FormSourceID
	}

	// For edit mode, load the site if not already present
	if mode, ok := data["Mode"].(FormMode); ok && mode == FormModeEdit {
		h.loadSiteForEdit(r, data)
	}

	h.renderSiteForm(w, r, data)
}

// loadSiteForEdit loads the site for edit mode if not already present in data.
func (h *UIHandlers) loadSiteForEdit(r *http.Request, data map[string]any) {
	if _, hasSite := data["SiteID"]; hasSite {
		return
	}
	id := r.PathValue("id")
	if id == "" || h.SiteSvc == nil {
		return
	}
	data["SiteID"] = id
}

// SiteCreate handles POST to create a site.
func (h *UIHandlers) SiteCreate(w http.ResponseWriter, r *http.Request) {
	if h.SiteSvc == nil {
		h.NotFound(w, r)
		return
	}

	HandleForm(FormHandlerOpts[siteFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parseSiteForm,
		Service:    &siteFormService{svc: h.SiteSvc},
		Renderer:   h.renderSiteFormWithData,
		SuccessURL: "/sites",
		PageMeta:   PageMeta{Title: "Merrymaker - New Site", PageTitle: "New Site", CurrentPage: PageSiteForm},
	})
}

// SiteUpdate handles POST to update an existing site.
func (h *UIHandlers) SiteUpdate(w http.ResponseWriter, r *http.Request) {
	if h.SiteSvc == nil {
		h.NotFound(w, r)
		return
	}

	HandleForm(FormHandlerOpts[siteFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeEdit,
		Parser:     parseSiteForm,
		Service:    &siteFormService{svc: h.SiteSvc},
		Renderer:   h.renderSiteFormWithData,
		SuccessURL: "/sites",
		PageMeta:   PageMeta{Title: "Merrymaker - Edit Site", PageTitle: "Edit Site", CurrentPage: PageSiteForm},
	})
}
