package httpx

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/http/validation"
)

// allowlistFormView groups form data to keep parameter count small.
type allowlistFormView struct {
	Data map[string]any
}

// renderAllowlistForm renders the allowlist form page with common data.
// Note: This function is domain-specific and not consolidated with other form renderers because:
// - It has unique page titles and CurrentPage values.
// - Scope-based allowlists don't require site options (unlike site-specific domains).
// - Consolidation would add complexity without meaningful code reduction.
func (h *UIHandlers) renderAllowlistForm(w http.ResponseWriter, r *http.Request, v allowlistFormView) {
	data, _ := prepareFormFrame(FormFrameOpts{
		R:           r,
		Data:        v.Data,
		DefaultMode: FormModeCreate,
		MetaForMode: func(mode FormMode) PageMeta {
			if mode == FormModeEdit {
				return PageMeta{
					Title:       "Merrymaker - Edit Domain Allow List",
					PageTitle:   "Edit Domain Allow List",
					CurrentPage: PageAllowlistForm,
				}
			}
			return PageMeta{
				Title:       "Merrymaker - New Domain Allow List",
				PageTitle:   "New Domain Allow List",
				CurrentPage: PageAllowlistForm,
			}
		},
	})
	// No site options needed for scope-based allowlists
	h.renderDashboardPage(w, r, data)
}

// buildSiteOptions is no longer needed for scope-based allowlists

// allowlistFormFields groups form fields to keep parameter count small.
type allowlistFormFields struct {
	Pattern, PatternType, Scope, Description string
	Priority                                 int
	Enabled                                  bool
}

// parseAllowlistFormFields extracts and validates form fields from the request.
func parseAllowlistFormFields(r *http.Request) (allowlistFormFields, map[string]string) {
	if err := r.ParseForm(); err != nil {
		return allowlistFormFields{}, map[string]string{"general": "Invalid form submission."}
	}

	fields := allowlistFormFields{
		Pattern:     strings.TrimSpace(r.FormValue("pattern")),
		PatternType: strings.TrimSpace(r.FormValue("pattern_type")),
		Scope:       strings.TrimSpace(r.FormValue("scope")),
		Description: strings.TrimSpace(r.FormValue("description")),
		Enabled:     r.FormValue("enabled") == "on",
	}

	errs := map[string]string{}

	// Parse priority
	priorityStr := strings.TrimSpace(r.FormValue("priority"))
	if priorityStr == "" {
		fields.Priority = 100 // default
	} else if p, err := strconv.Atoi(priorityStr); err != nil {
		errs["priority"] = "Priority must be a valid number."
	} else {
		fields.Priority = p
	}

	return fields, errs
}

// validateAllowlistForm validates allowlist form fields.
func validateAllowlistForm(fields allowlistFormFields) map[string]string {
	validPatternTypes := []string{
		model.PatternTypeExact,
		model.PatternTypeWildcard,
		model.PatternTypeGlob,
		model.PatternTypeETLDPlusOne,
	}

	v := validation.New().
		Validate("pattern", fields.Pattern, validation.Required("Domain pattern", 255)).
		Validate("pattern_type", fields.PatternType, validation.OneOf("Pattern type", validPatternTypes)).
		Validate("scope", fields.Scope, validation.Optional("Scope", 100)).
		Validate("description", fields.Description, validation.Optional("Description", 1000))

	// Custom validation for priority range
	if fields.Priority < 1 || fields.Priority > 1000 {
		v.Errors()["priority"] = "Priority must be between 1 and 1000."
	}

	return v.Errors()
}

// --- Handlers ---

// AllowlistNew renders the create form.
func (h *UIHandlers) AllowlistNew(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{
		"Mode":         FormModeCreate,
		"FormEnabled":  true,
		"FormPriority": 100,
	}
	h.renderAllowlistForm(w, r, allowlistFormView{Data: data})
}

// AllowlistEdit renders the edit form.
func (h *UIHandlers) AllowlistEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.NotFound(w, r)
		return
	}
	if h.AllowlistSvc == nil {
		h.NotFound(w, r)
		return
	}

	allowlist, err := h.AllowlistSvc.GetByID(r.Context(), id)
	if err != nil {
		h.NotFound(w, r)
		return
	}

	data := map[string]any{
		"Mode":            FormModeEdit,
		"AllowlistID":     id,
		"FormPattern":     allowlist.Pattern,
		"FormPatternType": allowlist.PatternType,
		"FormScope":       allowlist.Scope,
		"FormDescription": allowlist.Description,
		"FormPriority":    allowlist.Priority,
		"FormEnabled":     allowlist.Enabled,
	}
	h.renderAllowlistForm(w, r, allowlistFormView{Data: data})
}

// allowlistFormData holds parsed form data for allowlist creation and updates.
type allowlistFormData struct {
	Pattern     string
	PatternType string
	Scope       string
	Description string
	Priority    int
	Enabled     bool
	// Form state preservation
	FormPattern     string
	FormPatternType string
	FormScope       string
	FormDescription string
	FormPriority    int
	FormEnabled     bool
}

// parseAllowlistForm parses and validates allowlist form data.
func parseAllowlistForm(r *http.Request) (allowlistFormData, map[string]string) {
	fields, parseErrs := parseAllowlistFormFields(r)
	errs := validateAllowlistForm(fields)
	for k, v := range parseErrs {
		if v != "" {
			errs[k] = v
		}
	}

	return allowlistFormData{
		Pattern:         fields.Pattern,
		PatternType:     fields.PatternType,
		Scope:           fields.Scope,
		Description:     fields.Description,
		Priority:        fields.Priority,
		Enabled:         fields.Enabled,
		FormPattern:     fields.Pattern,
		FormPatternType: fields.PatternType,
		FormScope:       fields.Scope,
		FormDescription: fields.Description,
		FormPriority:    fields.Priority,
		FormEnabled:     fields.Enabled,
	}, errs
}

// allowlistFormService adapts DomainAllowlistsService to work with the generic form handler.
type allowlistFormService struct {
	svc DomainAllowlistsService
}

func (s *allowlistFormService) Create(ctx context.Context, req allowlistFormData) (any, error) {
	return s.svc.Create(ctx, &model.CreateDomainAllowlistRequest{
		Pattern:     req.Pattern,
		PatternType: req.PatternType,
		Scope:       req.Scope,
		Description: req.Description,
		Enabled:     &req.Enabled,
		Priority:    &req.Priority,
	})
}

func (s *allowlistFormService) Update(ctx context.Context, id string, req allowlistFormData) (any, error) {
	return s.svc.Update(ctx, id, model.UpdateDomainAllowlistRequest{
		Scope:       &req.Scope,
		Pattern:     &req.Pattern,
		PatternType: &req.PatternType,
		Description: &req.Description,
		Enabled:     &req.Enabled,
		Priority:    &req.Priority,
	})
}

// handleAllowlistError handles domain-specific errors for allowlist.
func handleAllowlistError(_ error) (map[string]string, string) {
	// No specific domain errors to handle yet
	return nil, ""
}

// renderAllowlistFormWithData is a wrapper that adapts the generic form handler data to the allowlist form renderer.
func (h *UIHandlers) renderAllowlistFormWithData(w http.ResponseWriter, r *http.Request, data map[string]any) {
	// Extract FormData if present and add individual fields for template compatibility
	if formData, ok := data["FormData"].(allowlistFormData); ok {
		data["FormPattern"] = formData.FormPattern
		data["FormPatternType"] = formData.FormPatternType
		data["FormScope"] = formData.FormScope
		data["FormDescription"] = formData.FormDescription
		data["FormPriority"] = formData.FormPriority
		data["FormEnabled"] = formData.FormEnabled
	}

	h.renderAllowlistForm(w, r, allowlistFormView{Data: data})
}

// AllowlistCreate handles POST from the create form.
func (h *UIHandlers) AllowlistCreate(w http.ResponseWriter, r *http.Request) {
	if h.AllowlistSvc == nil {
		h.NotFound(w, r)
		return
	}

	HandleForm(FormHandlerOpts[allowlistFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parseAllowlistForm,
		Service:    &allowlistFormService{svc: h.AllowlistSvc},
		Renderer:   h.renderAllowlistFormWithData,
		SuccessURL: "/allowlist",
		PageMeta: PageMeta{
			Title:       "Merrymaker - New Domain Allow List",
			PageTitle:   "New Domain Allow List",
			CurrentPage: PageAllowlistForm,
		},
		HandleError: handleAllowlistError,
	})
}

// AllowlistUpdate handles POST from the edit form.
func (h *UIHandlers) AllowlistUpdate(w http.ResponseWriter, r *http.Request) {
	if h.AllowlistSvc == nil {
		h.NotFound(w, r)
		return
	}

	HandleForm(FormHandlerOpts[allowlistFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeEdit,
		Parser:     parseAllowlistForm,
		Service:    &allowlistFormService{svc: h.AllowlistSvc},
		Renderer:   h.renderAllowlistFormWithData,
		SuccessURL: "/allowlist",
		PageMeta: PageMeta{
			Title:       "Merrymaker - Edit Domain Allow List",
			PageTitle:   "Edit Domain Allow List",
			CurrentPage: PageAllowlistForm,
		},
		HandleError: handleAllowlistError,
	})
}

// AllowlistDelete handles POST to delete an allowlist entry.
func (h *UIHandlers) AllowlistDelete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		h.NotFound(w, r)
		return
	}
	if h.AllowlistSvc == nil {
		h.NotFound(w, r)
		return
	}

	err := h.AllowlistSvc.Delete(r.Context(), id)
	if err != nil {
		// For delete errors, redirect back to list with error
		HTMX(w).Redirect("/allowlist?error=delete_failed")
		return
	}

	HTMX(w).Redirect("/allowlist")
}
