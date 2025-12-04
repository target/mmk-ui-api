// Package httpx provides HTTP handlers and utilities for the merrymaker job system API.
package httpx

import (
	"errors"
	"net/http"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
)

// HTTPAlertSinkHandlers provides HTTP handlers for HTTP alert sink operations.
type HTTPAlertSinkHandlers struct {
	Svc *service.HTTPAlertSinkService
}

const (
	defaultHTTPAlertSinkListLimit = 50  // Default number of sinks returned when limit is not specified
	maxHTTPAlertSinkListLimit     = 100 // Maximum number of sinks that can be requested in one call
)

// Create handles HTTP requests to create a new HTTP alert sink.
func (h *HTTPAlertSinkHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateHTTPAlertSinkRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	sink, err := h.Svc.Create(r.Context(), &req)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrHTTPAlertSinkNameExists):
			WriteError(w, ErrorParams{Code: http.StatusConflict, ErrCode: "name_conflict", Err: err})
		case isValidationError(err):
			WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "validation_failed", Err: err})
		default:
			WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "create_failed", Err: err})
		}
		return
	}

	WriteJSON(w, http.StatusCreated, sink)
}

// List handles HTTP requests to list HTTP alert sinks with pagination.
func (h *HTTPAlertSinkHandlers) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := ParseLimitOffset(r, defaultHTTPAlertSinkListLimit, maxHTTPAlertSinkListLimit)

	sinks, err := h.Svc.List(r.Context(), limit, offset)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "list_failed", Err: err})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"http_alert_sinks": sinks,
		"limit":            limit,
		"offset":           offset,
	})
}

// GetByID handles HTTP requests to get an HTTP alert sink by ID.
func (h *HTTPAlertSinkHandlers) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(
			w,
			ErrorParams{
				Code:    http.StatusBadRequest,
				ErrCode: "invalid_path",
				Err:     errors.New("http alert sink id is required"),
			},
		)
		return
	}

	sink, err := h.Svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, data.ErrHTTPAlertSinkNotFound) {
			WriteError(w, ErrorParams{Code: http.StatusNotFound, ErrCode: "http_alert_sink_not_found", Err: err})
			return
		}
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "get_failed", Err: err})
		return
	}

	WriteJSON(w, http.StatusOK, sink)
}

// Update handles HTTP requests to update an HTTP alert sink.

func (h *HTTPAlertSinkHandlers) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(
			w,
			ErrorParams{
				Code:    http.StatusBadRequest,
				ErrCode: "invalid_path",
				Err:     errors.New("http alert sink id is required"),
			},
		)
		return
	}

	var req *model.UpdateHTTPAlertSinkRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	sink, err := h.Svc.Update(r.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrHTTPAlertSinkNotFound):
			WriteError(w, ErrorParams{Code: http.StatusNotFound, ErrCode: "http_alert_sink_not_found", Err: err})
		case errors.Is(err, data.ErrHTTPAlertSinkNameExists):
			WriteError(w, ErrorParams{Code: http.StatusConflict, ErrCode: "name_conflict", Err: err})
		case isValidationError(err):
			WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "validation_failed", Err: err})
		default:
			WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "update_failed", Err: err})
		}
		return
	}

	WriteJSON(w, http.StatusOK, sink)
}

// Delete handles HTTP requests to delete an HTTP alert sink.
func (h *HTTPAlertSinkHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(
			w,
			ErrorParams{
				Code:    http.StatusBadRequest,
				ErrCode: "invalid_path",
				Err:     errors.New("http alert sink id is required"),
			},
		)
		return
	}

	deleted, err := h.Svc.Delete(r.Context(), id)
	if err != nil {
		if errors.Is(err, data.ErrHTTPAlertSinkNotFound) {
			WriteError(w, ErrorParams{Code: http.StatusNotFound, ErrCode: "http_alert_sink_not_found", Err: err})
			return
		}
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "delete_failed", Err: err})
		return
	}

	if !deleted {
		WriteError(
			w,
			ErrorParams{
				Code:    http.StatusNotFound,
				ErrCode: "http_alert_sink_not_found",
				Err:     errors.New("http alert sink not found"),
			},
		)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
