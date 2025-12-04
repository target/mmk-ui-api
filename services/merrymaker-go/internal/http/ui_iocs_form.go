package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// iocFormData holds parsed form data for IOC create/edit.
type iocFormData struct {
	Type        model.IOCType
	Value       string
	BulkValues  string // Newline-separated values for bulk import
	EntryMode   string // "single" or "bulk"
	Enabled     bool
	Description string
}

// IOCNew renders the create form for a new IOC.
func (h *UIHandlers) IOCNew(w http.ResponseWriter, r *http.Request) {
	if h.IOCSvc == nil {
		h.NotFound(w, r)
		return
	}

	data := map[string]any{
		"Mode":        FormModeCreate,
		"FormEnabled": true, // Default to enabled
	}
	h.renderIOCForm(w, r, data)
}

// IOCEdit renders the edit form for an existing IOC.
func (h *UIHandlers) IOCEdit(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" || h.IOCSvc == nil {
		h.NotFound(w, r)
		return
	}

	ioc, err := h.IOCSvc.GetByID(r.Context(), id)
	if err != nil || ioc == nil {
		h.NotFound(w, r)
		return
	}

	// Safely dereference pointer fields
	description := ""
	if ioc.Description != nil {
		description = *ioc.Description
	}

	data := map[string]any{
		"Mode":            FormModeEdit,
		"IOCID":           ioc.ID,
		"FormType":        string(ioc.Type),
		"FormValue":       ioc.Value,
		"FormEnabled":     ioc.Enabled,
		"FormDescription": description,
	}
	h.renderIOCForm(w, r, data)
}

// renderIOCForm renders the IOC form template with the given data.
func (h *UIHandlers) renderIOCForm(w http.ResponseWriter, r *http.Request, data map[string]any) {
	data, _ = prepareFormFrame(FormFrameOpts{
		R:           r,
		Data:        data,
		DefaultMode: FormModeCreate,
		MetaForMode: func(mode FormMode) PageMeta {
			title := "New IOC"
			if mode == FormModeEdit {
				title = "Edit IOC"
			}
			return PageMeta{
				Title:       "Merrymaker - " + title,
				PageTitle:   title,
				CurrentPage: PageIOCForm,
			}
		},
	})
	h.renderDashboardPage(w, r, data)
}

// renderIOCFormWithData is an adapter for the generic form handler.
func (h *UIHandlers) renderIOCFormWithData(w http.ResponseWriter, r *http.Request, data map[string]any) {
	// Extract FormData if present and add individual fields for template compatibility
	if formData, ok := data["FormData"].(iocFormData); ok {
		data["FormType"] = string(formData.Type)
		data["FormValue"] = formData.Value
		data["FormBulkValues"] = formData.BulkValues
		data["FormEnabled"] = formData.Enabled
		data["FormDescription"] = formData.Description
	}

	h.renderIOCForm(w, r, data)
}

// parseIOCForm parses the IOC form data from the request.
func parseIOCForm(r *http.Request) (iocFormData, map[string]string) {
	if err := r.ParseForm(); err != nil {
		return iocFormData{}, map[string]string{
			"form": "Unable to parse form data.",
		}
	}

	data := iocFormData{
		Type:        model.IOCType(strings.TrimSpace(r.FormValue("type"))),
		Value:       strings.TrimSpace(r.FormValue("value")),
		BulkValues:  strings.TrimSpace(r.FormValue("bulk_values")),
		EntryMode:   strings.TrimSpace(r.FormValue("entry_mode")),
		Enabled:     r.FormValue("enabled") == "1",
		Description: strings.TrimSpace(r.FormValue("description")),
	}

	// Client-side validation
	errors := make(map[string]string)

	if data.Type == "" {
		errors["type"] = "Type is required."
	} else if !data.Type.Valid() {
		errors["type"] = "Invalid IOC type. Must be 'fqdn' or 'ip'."
	}

	// Validate based on entry mode
	if data.EntryMode == "bulk" {
		validateBulkEntryMode(data.BulkValues, errors)
		return data, errors
	}

	validateSingleEntryMode(data.Value, errors)

	return data, errors
}

// iocFormService adapts IOCService to work with the generic form handler.
type iocFormService struct {
	svc IOCService
}

// parseBulkValues splits, trims, and deduplicates bulk IOC values.
func parseBulkValues(bulkValues string) ([]string, error) {
	lines := strings.Split(bulkValues, "\n")
	values := make([]string, 0, len(lines))
	seen := make(map[string]bool, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		values = append(values, trimmed)
	}

	if len(values) == 0 {
		return nil, errors.New("no valid IOC values provided")
	}

	if len(values) > maxBulkIOCs {
		return nil, fmt.Errorf("bulk import limited to %d IOCs, received %d", maxBulkIOCs, len(values))
	}

	return values, nil
}

func validateBulkEntryMode(bulkValues string, errors map[string]string) {
	if bulkValues != "" {
		return
	}

	errors["bulk_values"] = "At least one IOC value is required."
}

func validateSingleEntryMode(value string, errors map[string]string) {
	if value != "" {
		return
	}

	errors["value"] = "Value is required."
}

// iocCreateParams holds common parameters for IOC creation.
type iocCreateParams struct {
	enabled     *bool
	description *string
}

func (s *iocFormService) Create(ctx context.Context, req iocFormData) (any, error) {
	params := iocCreateParams{
		enabled: &req.Enabled,
	}
	if req.Description != "" {
		params.description = &req.Description
	}

	if req.EntryMode == "bulk" {
		return s.createBulk(ctx, req, params)
	}

	return s.createSingle(ctx, req, params)
}

func (s *iocFormService) createBulk(ctx context.Context, req iocFormData, params iocCreateParams) (any, error) {
	values, err := parseBulkValues(req.BulkValues)
	if err != nil {
		return nil, err
	}

	bulkReq := model.BulkCreateIOCsRequest{
		Type:        req.Type,
		Values:      values,
		Enabled:     params.enabled,
		Description: params.description,
	}

	count, err := s.svc.BulkCreate(ctx, bulkReq)
	if err != nil {
		return nil, fmt.Errorf("bulk create IOCs: %w", err)
	}

	return &model.IOC{
		ID:    fmt.Sprintf("bulk-%d", count),
		Type:  req.Type,
		Value: fmt.Sprintf("%d IOCs imported", count),
	}, nil
}

func (s *iocFormService) createSingle(
	ctx context.Context,
	req iocFormData,
	params iocCreateParams,
) (*model.IOC, error) {
	createReq := model.CreateIOCRequest{
		Type:        req.Type,
		Value:       req.Value,
		Enabled:     params.enabled,
		Description: params.description,
	}

	return s.svc.Create(ctx, createReq)
}

func (s *iocFormService) Update(ctx context.Context, id string, req iocFormData) (any, error) {
	enabled := req.Enabled
	// Always set description pointer to allow clearing (empty string clears the field)
	description := req.Description

	updateReq := model.UpdateIOCRequest{
		Type:        &req.Type,
		Value:       &req.Value,
		Enabled:     &enabled,
		Description: &description,
	}

	return s.svc.Update(ctx, id, updateReq)
}

// handleIOCError handles domain-specific errors for IOCs.
func handleIOCError(err error) (map[string]string, string) {
	if errors.Is(err, data.ErrIOCAlreadyExists) {
		return map[string]string{"value": "An IOC with this value already exists."}, ""
	}
	if errors.Is(err, data.ErrIOCNotFound) {
		return nil, "Unable to update IOC. Please try again."
	}
	// Return nil to let the default handler take over
	return nil, ""
}

// IOCCreate handles POST from the create form.
func (h *UIHandlers) IOCCreate(w http.ResponseWriter, r *http.Request) {
	if h.IOCSvc == nil {
		h.NotFound(w, r)
		return
	}

	HandleForm(FormHandlerOpts[iocFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parseIOCForm,
		Service:    &iocFormService{svc: h.IOCSvc},
		Renderer:   h.renderIOCFormWithData,
		SuccessURL: "/iocs",
		PageMeta: PageMeta{
			Title:       "Merrymaker - New IOC",
			PageTitle:   "New IOC",
			CurrentPage: PageIOCForm,
		},
		ExtraData:   map[string]any{},
		HandleError: handleIOCError,
	})
}

// IOCUpdate handles POST from the edit form.
func (h *UIHandlers) IOCUpdate(w http.ResponseWriter, r *http.Request) {
	if h.IOCSvc == nil {
		h.NotFound(w, r)
		return
	}

	HandleForm(FormHandlerOpts[iocFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeEdit,
		Parser:     parseIOCForm,
		Service:    &iocFormService{svc: h.IOCSvc},
		Renderer:   h.renderIOCFormWithData,
		SuccessURL: "/iocs",
		PageMeta: PageMeta{
			Title:       "Merrymaker - Edit IOC",
			PageTitle:   "Edit IOC",
			CurrentPage: PageIOCForm,
		},
		ExtraData:   map[string]any{},
		HandleError: handleIOCError,
	})
}

// IOCDelete handles POST to delete an IOC.
func (h *UIHandlers) IOCDelete(w http.ResponseWriter, r *http.Request) {
	if h.IOCSvc == nil {
		h.NotFound(w, r)
		return
	}

	id := r.PathValue("id")
	if id == "" {
		h.NotFound(w, r)
		return
	}

	deleted, err := h.IOCSvc.Delete(r.Context(), id)
	if err != nil || !deleted {
		data := map[string]any{
			"Error":       "Unable to delete IOC. Please try again.",
			"IOCs":        []*model.IOC{},
			"HasNext":     false,
			"Title":       "Merrymaker - IOCs",
			"PageTitle":   "IOCs",
			"CurrentPage": PageIOCs,
		}
		h.renderDashboardPage(w, r, data)
		return
	}

	// Redirect to list view
	http.Redirect(w, r, "/iocs", http.StatusSeeOther)
}
