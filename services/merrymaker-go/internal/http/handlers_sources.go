// Package httpx provides HTTP handlers and utilities for the merrymaker job system API.
package httpx

import (
	"errors"
	"net/http"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
)

// SourceHandlers provides HTTP handlers for source-related operations using the orchestration service layer.
type SourceHandlers struct {
	Svc *service.SourceService
}

// Create handles HTTP requests to create a new source. If Test=true, a job is auto-enqueued.
func (h *SourceHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var req *model.CreateSourceRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	src, err := h.Svc.Create(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrSourceNameExists):
			WriteError(w, ErrorParams{Code: http.StatusConflict, ErrCode: "name_conflict", Err: err})
		case isValidationError(err):
			WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "validation_failed", Err: err})
		default:
			WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "create_failed", Err: err})
		}
		return
	}

	WriteJSON(w, http.StatusCreated, src)
}
