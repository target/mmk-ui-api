package httpx

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/http/validation"
	"github.com/target/mmk-ui-api/internal/service"
)

// Secrets serves the secrets list page, HTMX-aware.
func (h *UIHandlers) Secrets(w http.ResponseWriter, r *http.Request) {
	// Use generic list handler - no filtering needed for secrets
	HandleList(ListHandlerOpts[*model.Secret, struct{}]{
		Handler: h,
		W:       w,
		R:       r,
		Fetcher: func(ctx context.Context, pg pageOpts) ([]*model.Secret, error) {
			// Fetch pageSize+1 to detect hasNext
			limit, offset := pg.LimitAndOffset()
			secrets, err := h.SecretSvc.List(ctx, limit, offset)
			if err != nil {
				h.logger().Error("failed to load secrets for UI",
					"error", err,
					"page", pg.Page,
					"page_size", pg.PageSize,
				)
			}
			return secrets, err
		},
		BasePath: "/secrets",
		PageMeta: PageMeta{
			Title:       "Merrymaker - Secrets",
			PageTitle:   "Secrets",
			CurrentPage: PageSecrets,
		},
		ItemsKey:     "Secrets",
		ErrorMessage: "Unable to load secrets.",
		ServiceAvailable: func() bool {
			return h.SecretSvc != nil
		},
		UnavailableMessage: "Unable to load secrets.",
	})
}

// --- Secrets Create/Edit (UI) ---

// secretNameRe validates secret names.
var secretNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)

func (h *UIHandlers) renderSecretForm(w http.ResponseWriter, r *http.Request, data map[string]any) {
	data, _ = prepareFormFrame(FormFrameOpts{
		R:           r,
		Data:        data,
		DefaultMode: FormModeCreate,
		MetaForMode: func(mode FormMode) PageMeta {
			if mode == FormModeEdit {
				return PageMeta{
					Title:       "Merrymaker - Edit Secret",
					PageTitle:   "Edit Secret",
					CurrentPage: PageSecretForm,
				}
			}
			return PageMeta{Title: "Merrymaker - New Secret", PageTitle: "New Secret", CurrentPage: PageSecretForm}
		},
	})
	h.renderDashboardPage(w, r, data)
}

// validateSecretName validates secret name with custom error message.
func validateSecretName(name string) string {
	name = strings.TrimSpace(name)
	if name == "" || !secretNameRe.MatchString(name) || len(name) > 255 {
		return "Use letters, digits, underscore, and hyphens. Max 255 characters."
	}
	return ""
}

// validateSecretValue validates secret value with custom error message.
func validateSecretValue(value string) string {
	if strings.TrimSpace(value) == "" {
		return "Secret is required."
	}
	if len(value) > 10000 {
		return "Secret cannot exceed 10000 characters."
	}
	return ""
}

// validateSecret enforces name rules and optional value requirement.
func validateSecret(name, value string, requireValue bool) map[string]string {
	v := validation.New().
		Validate("name", name, validateSecretName)

	// Validate value if required OR if provided (even when optional)
	if requireValue || strings.TrimSpace(value) != "" {
		v.Validate("value", value, validateSecretValue)
	}
	return v.Errors()
}

// validateProviderScriptPath validates provider script path for dynamic secrets.
func validateProviderScriptPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return "Provider script path is required for dynamic secrets."
	}
	if len(path) > 500 {
		return "Provider script path cannot exceed 500 characters."
	}
	return ""
}

// validateEnvConfig validates environment configuration JSON.
func validateEnvConfig(envConfig string) string {
	envConfig = strings.TrimSpace(envConfig)
	if envConfig == "" {
		return "" // Optional field
	}
	if len(envConfig) > 10000 {
		return "Environment config cannot exceed 10000 characters."
	}
	// Validate JSON format
	var temp map[string]string
	if err := json.Unmarshal([]byte(envConfig), &temp); err != nil {
		return "Environment config must be valid JSON object with string values."
	}
	// Validate keys are non-empty after trimming
	for k := range temp {
		if strings.TrimSpace(k) == "" {
			return "Environment variable names cannot be empty."
		}
	}
	return ""
}

// validateRefreshInterval validates refresh interval provided in minutes.
func validateRefreshInterval(intervalStr string) string {
	intervalStr = strings.TrimSpace(intervalStr)
	if intervalStr == "" {
		return "Refresh interval is required for dynamic secrets."
	}
	minutes, err := strconv.ParseFloat(intervalStr, 64)
	if err != nil {
		return "Refresh interval must be a valid number."
	}
	if minutes < 1 {
		return "Refresh interval must be at least 1 minute."
	}
	if minutes > 10080 { // 7 days
		return "Refresh interval cannot exceed 7 days (10080 minutes)."
	}
	return ""
}

// minutesStringToSeconds converts a string containing minutes into seconds.
func minutesStringToSeconds(intervalStr string) (int64, error) {
	minutes, err := strconv.ParseFloat(intervalStr, 64)
	if err != nil {
		return 0, err
	}
	seconds := int64(math.Round(minutes * 60))
	return seconds, nil
}

// formatSecondsAsMinutes renders seconds in minutes using integers where possible.
func formatSecondsAsMinutes(seconds int64) string {
	if seconds <= 0 {
		return ""
	}
	if seconds%60 == 0 {
		return strconv.FormatInt(seconds/60, 10)
	}
	minutes := float64(seconds) / 60
	s := strconv.FormatFloat(minutes, 'f', 3, 64)
	s = strings.TrimRight(strings.TrimRight(s, "0"), ".")
	return s
}

type secretValidationInput struct {
	Name               string
	Value              string
	RequireValue       bool
	IsCreate           bool // True for create mode, false for update mode
	ProviderScriptPath string
	EnvConfig          string
	RefreshInterval    string
	RefreshEnabled     bool
}

// validateSecretWithRefresh enforces validation rules for secrets with optional refresh configuration.
func validateSecretWithRefresh(in secretValidationInput) map[string]string {
	errs := validateSecret(in.Name, in.Value, shouldRequireValue(in))

	// Validate refresh configuration if enabled
	if in.RefreshEnabled {
		if err := validateProviderScriptPath(in.ProviderScriptPath); err != "" {
			errs["provider_script_path"] = err
		}
		if err := validateRefreshInterval(in.RefreshInterval); err != "" {
			errs["refresh_interval_minutes"] = err
		}
	}

	// Always validate env_config if provided (even when refresh is disabled)
	if strings.TrimSpace(in.EnvConfig) != "" {
		if err := validateEnvConfig(in.EnvConfig); err != "" {
			errs["env_config"] = err
		}
	}

	return errs
}

// shouldRequireValue determines if the secret value is required based on the validation context.
// Returns true if:
// - For update mode: RequireValue is true (replace=true checkbox was checked).
// - For create mode: refresh is NOT enabled (static secrets require a value).
func shouldRequireValue(in secretValidationInput) bool {
	if in.IsCreate {
		return !in.RefreshEnabled
	}
	return in.RequireValue
}

// SecretNew renders the create form.
func (h *UIHandlers) SecretNew(w http.ResponseWriter, r *http.Request) {
	data := map[string]any{"Mode": "create"}
	h.renderSecretForm(w, r, data)
}

// SecretEdit renders the edit form for an existing secret (masked value).
func (h *UIHandlers) SecretEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || h.SecretSvc == nil {
		h.NotFound(w, r)
		return
	}
	sec, err := h.SecretSvc.GetByID(r.Context(), id)
	if err != nil || sec == nil {
		h.NotFound(w, r)
		return
	}

	// Build secret data with refresh configuration
	secretData := h.buildSecretData(sec)

	data := map[string]any{
		"Mode":   "edit",
		"Secret": secretData,
	}
	h.renderSecretForm(w, r, data)
}

// buildSecretData builds template data from a secret entity.
func (h *UIHandlers) buildSecretData(sec *model.Secret) map[string]any {
	secretData := map[string]any{
		"ID":   sec.ID,
		"Name": sec.Name,
	}

	// Add refresh configuration fields
	secretData["RefreshEnabled"] = sec.RefreshEnabled
	if sec.ProviderScriptPath != nil {
		secretData["ProviderScriptPath"] = *sec.ProviderScriptPath
	}
	if len(sec.EnvConfig) > 0 {
		secretData["EnvConfig"] = string(sec.EnvConfig)
	}
	if sec.RefreshInterval != nil {
		secretData["RefreshInterval"] = formatSecondsAsMinutes(*sec.RefreshInterval)
	}
	if sec.LastRefreshedAt != nil {
		secretData["LastRefreshedAt"] = *sec.LastRefreshedAt
	}
	if sec.LastRefreshStatus != nil {
		secretData["LastRefreshStatus"] = *sec.LastRefreshStatus
	}
	if sec.LastRefreshError != nil {
		secretData["LastRefreshError"] = *sec.LastRefreshError
	}

	return secretData
}

// secretFormData holds parsed form data for both create and update operations.
type secretFormData struct {
	Name     string
	Value    string
	Replace  bool // For update: whether to replace the value
	SecretID string
	FormName string // For preserving form state

	// Refresh configuration fields
	ProviderScriptPath  string
	EnvConfig           string
	RefreshInterval     string
	RefreshEnabled      bool
	FormProviderScript  string // For preserving form state
	FormEnvConfig       string // For preserving form state
	FormRefreshInterval string // For preserving form state
	FormRefreshEnabled  bool   // For preserving form state
}

// parseSecretForm parses and validates secret form data.
func parseSecretForm(r *http.Request) (secretFormData, map[string]string) {
	if err := r.ParseForm(); err != nil {
		return secretFormData{}, map[string]string{"_form": "Invalid form submission."}
	}

	name := strings.TrimSpace(r.Form.Get("name"))
	value := r.Form.Get("value")
	replace := r.Form.Get("replace") == "on" || r.Form.Get("replace") == "1"

	// Parse refresh configuration fields
	providerScriptPath := strings.TrimSpace(r.Form.Get("provider_script_path"))
	envConfig := strings.TrimSpace(r.Form.Get("env_config"))
	refreshInterval := strings.TrimSpace(r.Form.Get("refresh_interval_minutes"))
	refreshEnabled := r.Form.Get("refresh_enabled") == "on" || r.Form.Get("refresh_enabled") == "1"

	// Determine mode and value requirement
	isCreate := r.PathValue("id") == ""
	requireValue := isCreate || replace

	// Validate
	errs := validateSecretWithRefresh(secretValidationInput{
		Name:               name,
		Value:              value,
		RequireValue:       requireValue,
		IsCreate:           isCreate,
		ProviderScriptPath: providerScriptPath,
		EnvConfig:          envConfig,
		RefreshInterval:    refreshInterval,
		RefreshEnabled:     refreshEnabled,
	})

	return secretFormData{
		Name:                name,
		Value:               value,
		Replace:             replace,
		FormName:            name,
		ProviderScriptPath:  providerScriptPath,
		EnvConfig:           envConfig,
		RefreshInterval:     refreshInterval,
		RefreshEnabled:      refreshEnabled,
		FormProviderScript:  providerScriptPath,
		FormEnvConfig:       envConfig,
		FormRefreshInterval: refreshInterval,
		FormRefreshEnabled:  refreshEnabled,
	}, errs
}

// secretFormService adapts SecretsService to work with the generic form handler.
type secretFormService struct {
	svc SecretsService
}

func (s *secretFormService) Create(ctx context.Context, req secretFormData) (any, error) {
	createReq := model.CreateSecretRequest{
		Name:  req.Name,
		Value: req.Value,
	}

	// Add refresh configuration if enabled
	if req.RefreshEnabled {
		s.applyRefreshConfig(&createReq, req)
	}

	return s.svc.Create(ctx, createReq)
}

// applyRefreshConfig applies refresh configuration from form data to create request.
func (s *secretFormService) applyRefreshConfig(req *model.CreateSecretRequest, formData secretFormData) {
	req.RefreshEnabled = &formData.RefreshEnabled
	if formData.ProviderScriptPath != "" {
		req.ProviderScriptPath = &formData.ProviderScriptPath
	}
	if formData.EnvConfig != "" {
		req.EnvConfig = &formData.EnvConfig
	}
	// Parse and set refresh interval if provided
	if formData.RefreshInterval == "" {
		return
	}
	seconds, err := minutesStringToSeconds(formData.RefreshInterval)
	if err == nil {
		req.RefreshInterval = &seconds
	}
}

// applyRefreshConfigUpdate applies refresh configuration from form data to update request.
func (s *secretFormService) applyRefreshConfigUpdate(req *model.UpdateSecretRequest, formData secretFormData) {
	// Always set RefreshEnabled so the server can toggle it
	req.RefreshEnabled = &formData.RefreshEnabled

	// Provider script path: set when provided; if disabling and explicitly provided, allow clearing via empty string
	if formData.ProviderScriptPath != "" ||
		(!formData.RefreshEnabled && strings.TrimSpace(formData.ProviderScriptPath) == "") {
		req.ProviderScriptPath = &formData.ProviderScriptPath
	}
	// Env config: only set when provided with non-empty content to avoid invalid JSON (empty string)
	if strings.TrimSpace(formData.EnvConfig) != "" {
		req.EnvConfig = &formData.EnvConfig
	}

	// Refresh interval: when enabled and provided, parse and set.
	// When disabling refresh, do not include it so the repo leaves it unchanged (or cleared by a separate path if needed).
	if !formData.RefreshEnabled {
		return
	}
	if formData.RefreshInterval == "" {
		return
	}
	seconds, err := minutesStringToSeconds(formData.RefreshInterval)
	if err == nil {
		req.RefreshInterval = &seconds
	}
}

func (s *secretFormService) Update(
	ctx context.Context,
	id string,
	req secretFormData,
) (any, error) {
	var valuePtr *string
	if req.Replace {
		v := req.Value
		valuePtr = &v
	}

	updateReq := model.UpdateSecretRequest{
		Name:  &req.Name,
		Value: valuePtr,
	}

	// Add refresh configuration updates
	s.applyRefreshConfigUpdate(&updateReq, req)

	return s.svc.Update(ctx, id, updateReq)
}

// handleSecretError handles domain-specific errors for secrets.
func handleSecretError(err error) (map[string]string, string) {
	if errors.Is(err, data.ErrSecretNameExists) {
		return map[string]string{"name": "A secret with this name already exists."}, ""
	}
	var scriptErr *service.SecretProviderScriptError
	if errors.As(err, &scriptErr) {
		return nil, scriptErr.UserMessage()
	}
	if errors.Is(err, data.ErrSecretNotFound) {
		return nil, "Unable to update secret. Please try again."
	}
	// Return nil to let the default handler take over
	return nil, ""
}

// loadSecretForEdit loads the secret for edit mode if not already present in data.
func (h *UIHandlers) loadSecretForEdit(r *http.Request, data map[string]any) {
	if _, hasSecret := data["Secret"]; hasSecret {
		return
	}
	id := r.PathValue("id")
	if id == "" || h.SecretSvc == nil {
		return
	}
	sec, err := h.SecretSvc.GetByID(r.Context(), id)
	if err == nil && sec != nil {
		data["Secret"] = map[string]any{"ID": sec.ID, "Name": sec.Name}
		return
	}
	// If we can't load the secret, use a placeholder with the ID
	// This prevents template errors when rendering error messages
	data["Secret"] = map[string]any{"ID": id, "Name": ""}
}

// renderSecretFormWithData is a wrapper that adapts the generic form handler data to the secret form renderer.
func (h *UIHandlers) renderSecretFormWithData(
	w http.ResponseWriter,
	r *http.Request,
	data map[string]any,
) {
	// Extract FormData if present and add individual fields for template compatibility
	if formData, formOK := data["FormData"].(secretFormData); formOK {
		data["FormName"] = formData.FormName
	}

	// For edit mode, load the secret if not already present
	if mode, modeFormOK := data["Mode"].(FormMode); modeFormOK && mode == FormModeEdit {
		h.loadSecretForEdit(r, data)
	} else if modeStr, modeStringOK := data["Mode"].(string); modeStringOK && FormMode(modeStr) == FormModeEdit {
		// Fallback for string mode values
		h.loadSecretForEdit(r, data)
	}

	h.renderSecretForm(w, r, data)
}

// SecretCreate handles POST from the create form.
func (h *UIHandlers) SecretCreate(w http.ResponseWriter, r *http.Request) {
	if h.SecretSvc == nil {
		h.NotFound(w, r)
		return
	}

	HandleForm(FormHandlerOpts[secretFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parseSecretForm,
		Service:    &secretFormService{svc: h.SecretSvc},
		Renderer:   h.renderSecretFormWithData,
		SuccessURL: "/secrets",
		PageMeta: PageMeta{
			Title:       "Merrymaker - New Secret",
			PageTitle:   "New Secret",
			CurrentPage: PageSecretForm,
		},
		ExtraData:   map[string]any{},
		HandleError: handleSecretError,
	})
}

// SecretUpdate handles POST from the edit form.
func (h *UIHandlers) SecretUpdate(w http.ResponseWriter, r *http.Request) {
	if h.SecretSvc == nil {
		h.NotFound(w, r)
		return
	}

	HandleForm(FormHandlerOpts[secretFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeEdit,
		Parser:     parseSecretForm,
		Service:    &secretFormService{svc: h.SecretSvc},
		Renderer:   h.renderSecretFormWithData,
		SuccessURL: "/secrets",
		PageMeta: PageMeta{
			Title:       "Merrymaker - Edit Secret",
			PageTitle:   "Edit Secret",
			CurrentPage: PageSecretForm,
		},
		ExtraData:   map[string]any{},
		HandleError: handleSecretError,
	})
}

// SecretDelete handles deleting a secret from the UI, respecting FK constraints.
func (h *UIHandlers) SecretDelete(w http.ResponseWriter, r *http.Request) {
	h.handleDelete(w, r, deleteHandlerOpts{
		ServiceAvailable: func() bool { return h.SecretSvc != nil },
		Delete: func(ctx context.Context, id string) (bool, error) {
			return h.SecretSvc.Delete(ctx, id)
		},
		RedirectPath: "/secrets",
		OnError: func(w http.ResponseWriter, r *http.Request, err error) {
			h.handleSecretDeleteError(w, r, err)
		},
		OnSuccess: func(w http.ResponseWriter, r *http.Request, _ bool) {
			// For HTMX requests, trigger success toast and return empty content to remove the row
			if IsHTMX(r) {
				triggerToast(w, "Secret deleted successfully", "success")
				w.WriteHeader(http.StatusOK)
				// Return empty content - the row will be swapped out with nothing (removed)
				return
			}
			// For non-HTMX requests, redirect
			http.Redirect(w, r, "/secrets", http.StatusSeeOther)
		},
	})
}

// handleSecretDeleteError handles errors from secret deletion.
// For HTMX requests, it triggers a toast notification and returns 204 No Content to prevent swap.
// For non-HTMX requests, it re-renders the list with an error message.
func (h *UIHandlers) handleSecretDeleteError(w http.ResponseWriter, r *http.Request, err error) {
	// Get user-friendly error message
	errMsg := processError(err, nil)
	if errMsg == "" {
		errMsg = "Unable to delete secret. Please try again."
	}

	// For HTMX requests, trigger error toast and keep the row (204 prevents swap)
	if IsHTMX(r) {
		triggerToast(w, errMsg, "error")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// For non-HTMX requests, re-render the list with error message
	h.renderSecretListError(w, r, err)
}

// renderSecretListError renders the secret list page with an error message.
func (h *UIHandlers) renderSecretListError(w http.ResponseWriter, r *http.Request, err error) {
	page, pageSize := getPageParams(r.URL.Query())
	additionalData := h.buildSecretListData(r.Context(), page, pageSize)
	status := DetermineErrorStatus(err)

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      err,
		Renderer: h.renderDashboardPage,
		PageMeta: PageMeta{
			Title:       "Merrymaker - Secrets",
			PageTitle:   "Secrets",
			CurrentPage: "secrets",
		},
		Data:       additionalData,
		StatusCode: status,
	})
}

// buildSecretListData builds the data map for the secret list page.
func (h *UIHandlers) buildSecretListData(ctx context.Context, page, pageSize int) map[string]any {
	items, hasPrev, hasNext, start, end, listErr := paginate(
		ctx,
		pageOpts{Page: page, PageSize: pageSize},
		func(ctx context.Context, limit, offset int) ([]*model.Secret, error) {
			return h.SecretSvc.List(ctx, limit, offset)
		},
	)
	if listErr != nil {
		return nil
	}

	data := map[string]any{
		"Secrets":    items,
		"HasPrev":    hasPrev,
		"HasNext":    hasNext,
		"StartIndex": start,
		"EndIndex":   end,
		"Page":       page,
		"PageSize":   pageSize,
	}
	if hasPrev {
		data["PrevURL"] = buildPageURL("/secrets", url.Values{}, pageOpts{Page: page - 1, PageSize: pageSize})
	}
	if hasNext {
		data["NextURL"] = buildPageURL("/secrets", url.Values{}, pageOpts{Page: page + 1, PageSize: pageSize})
	}
	return data
}
