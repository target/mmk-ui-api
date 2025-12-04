package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

const (
	maxIOCListLimit = 500
	maxBulkIOCs     = 10000
)

// IOCService defines the interface for IOC operations.
type IOCService interface {
	Create(ctx context.Context, req model.CreateIOCRequest) (*model.IOC, error)
	GetByID(ctx context.Context, id string) (*model.IOC, error)
	List(ctx context.Context, opts model.IOCListOptions) ([]*model.IOC, error)
	Update(ctx context.Context, id string, req model.UpdateIOCRequest) (*model.IOC, error)
	Delete(ctx context.Context, id string) (bool, error)
	BulkCreate(ctx context.Context, req model.BulkCreateIOCsRequest) (int, error)
	Stats(ctx context.Context) (*core.IOCStats, error)
}

// IOCHandlers provides HTTP handlers for IOC API endpoints.
type IOCHandlers struct {
	Svc IOCService
}

// Create handles HTTP requests to create a new IOC.
func (h *IOCHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateIOCRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	ioc, err := h.Svc.Create(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrIOCAlreadyExists):
			WriteError(w, ErrorParams{Code: http.StatusConflict, ErrCode: "ioc_exists", Err: err})
		case isValidationError(err):
			WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "validation_failed", Err: err})
		default:
			WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "create_failed", Err: err})
		}
		return
	}

	WriteJSON(w, http.StatusCreated, ioc)
}

// List handles HTTP requests to list IOCs with filtering and pagination.
func (h *IOCHandlers) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := ParseLimitOffset(r, 50, maxIOCListLimit)

	// Parse filters from query params
	opts := model.IOCListOptions{
		Limit:  limit,
		Offset: offset,
	}

	// Type filter (validate early to return 400 instead of 500)
	if typeStr := r.URL.Query().Get("type"); typeStr != "" {
		iocType := model.IOCType(typeStr)
		if !iocType.Valid() {
			WriteError(
				w,
				ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_type", Err: errors.New("invalid IOC type")},
			)
			return
		}
		opts.Type = &iocType
	}

	// Enabled filter
	if enabledStr := r.URL.Query().Get("enabled"); enabledStr != "" {
		enabled := enabledStr == StrTrue || enabledStr == "1"
		opts.Enabled = &enabled
	}

	// Search filter
	if search := r.URL.Query().Get("q"); search != "" {
		opts.Search = &search
	}

	iocs, err := h.Svc.List(r.Context(), opts)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "list_failed", Err: err})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"iocs":   iocs,
		"limit":  limit,
		"offset": offset,
	})
}

// GetByID handles HTTP requests to get an IOC by ID.
func (h *IOCHandlers) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("ioc id is required")},
		)
		return
	}

	ioc, err := h.Svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, data.ErrIOCNotFound) {
			WriteError(w, ErrorParams{Code: http.StatusNotFound, ErrCode: "ioc_not_found", Err: err})
			return
		}
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "get_failed", Err: err})
		return
	}

	WriteJSON(w, http.StatusOK, ioc)
}

// Update handles HTTP requests to update an IOC.
//
//nolint:dupl // Standard CRUD update pattern; duplication is acceptable
func (h *IOCHandlers) Update(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("ioc id is required")},
		)
		return
	}

	var req model.UpdateIOCRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	ioc, err := h.Svc.Update(r.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrIOCNotFound):
			WriteError(w, ErrorParams{Code: http.StatusNotFound, ErrCode: "ioc_not_found", Err: err})
		case errors.Is(err, data.ErrIOCAlreadyExists):
			WriteError(w, ErrorParams{Code: http.StatusConflict, ErrCode: "ioc_exists", Err: err})
		case isValidationError(err):
			WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "validation_failed", Err: err})
		default:
			WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "update_failed", Err: err})
		}
		return
	}

	WriteJSON(w, http.StatusOK, ioc)
}

// Delete handles HTTP requests to delete an IOC.
func (h *IOCHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("ioc id is required")},
		)
		return
	}

	deleted, err := h.Svc.Delete(r.Context(), id)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "delete_failed", Err: err})
		return
	}

	if !deleted {
		WriteError(
			w,
			ErrorParams{Code: http.StatusNotFound, ErrCode: "ioc_not_found", Err: errors.New("ioc not found")},
		)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}

// BulkCreate handles HTTP requests to create multiple IOCs in bulk.
func (h *IOCHandlers) BulkCreate(w http.ResponseWriter, r *http.Request) {
	var req model.BulkCreateIOCsRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	// Guard against excessive payloads
	if len(req.Values) > maxBulkIOCs {
		WriteError(
			w,
			ErrorParams{
				Code:    http.StatusBadRequest,
				ErrCode: "too_many_values",
				Err:     fmt.Errorf("maximum %d IOCs allowed per bulk request", maxBulkIOCs),
			},
		)
		return
	}

	count, err := h.Svc.BulkCreate(r.Context(), req)
	if err != nil {
		switch {
		case isValidationError(err):
			WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "validation_failed", Err: err})
		default:
			WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "bulk_create_failed", Err: err})
		}
		return
	}

	WriteJSON(w, http.StatusCreated, map[string]any{
		"created": count,
		"total":   len(req.Values),
	})
}

// Stats handles HTTP requests to get IOC statistics.
func (h *IOCHandlers) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.Svc.Stats(r.Context())
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "stats_failed", Err: err})
		return
	}

	WriteJSON(w, http.StatusOK, stats)
}
