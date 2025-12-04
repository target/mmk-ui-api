// Package httpx provides HTTP handlers and utilities for the merrymaker job system API.
package httpx

import (
	"errors"
	"net/http"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
)

// SecretHandlers provides HTTP handlers for secret-related operations.
type SecretHandlers struct {
	Svc *service.SecretService
}

const (
	maxSecretListLimit = 100 // Maximum number of secrets that can be requested in one call
)

// Create handles HTTP requests to create a new secret.
func (h *SecretHandlers) Create(w http.ResponseWriter, r *http.Request) {
	var req model.CreateSecretRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	secret, err := h.Svc.Create(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrSecretNameExists):
			WriteError(w, ErrorParams{Code: http.StatusConflict, ErrCode: "name_conflict", Err: err})
		case isValidationError(err):
			WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "validation_failed", Err: err})
		default:
			WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "create_failed", Err: err})
		}
		return
	}

	WriteJSON(w, http.StatusCreated, secret)
}

// List handles HTTP requests to list secrets with pagination.
func (h *SecretHandlers) List(w http.ResponseWriter, r *http.Request) {
	limit, offset := ParseLimitOffset(r, 50, maxSecretListLimit)

	secrets, err := h.Svc.List(r.Context(), limit, offset)
	if err != nil {
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "list_failed", Err: err})
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"secrets": secrets,
		"limit":   limit,
		"offset":  offset,
	})
}

// GetByID handles HTTP requests to get a secret by ID.
func (h *SecretHandlers) GetByID(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("secret id is required")},
		)
		return
	}

	secret, err := h.Svc.GetByID(r.Context(), id)
	if err != nil {
		if errors.Is(err, data.ErrSecretNotFound) {
			WriteError(w, ErrorParams{Code: http.StatusNotFound, ErrCode: "secret_not_found", Err: err})
			return
		}
		WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "get_failed", Err: err})
		return
	}

	WriteJSON(w, http.StatusOK, secret)
}

// Update handles HTTP requests to update a secret.
//
//nolint:dupl // mirrors Create handler to share validation flow
func (h *SecretHandlers) Update(
	w http.ResponseWriter,
	r *http.Request,
) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("secret id is required")},
		)
		return
	}

	var req model.UpdateSecretRequest
	if !DecodeJSON(w, r, &req) {
		return
	}

	secret, err := h.Svc.Update(r.Context(), id, req)
	if err != nil {
		switch {
		case errors.Is(err, data.ErrSecretNotFound):
			WriteError(w, ErrorParams{Code: http.StatusNotFound, ErrCode: "secret_not_found", Err: err})
		case errors.Is(err, data.ErrSecretNameExists):
			WriteError(w, ErrorParams{Code: http.StatusConflict, ErrCode: "name_conflict", Err: err})
		case isValidationError(err):
			WriteError(w, ErrorParams{Code: http.StatusBadRequest, ErrCode: "validation_failed", Err: err})
		default:
			WriteError(w, ErrorParams{Code: http.StatusInternalServerError, ErrCode: "update_failed", Err: err})
		}
		return
	}

	WriteJSON(w, http.StatusOK, secret)
}

// Delete handles HTTP requests to delete a secret.
func (h *SecretHandlers) Delete(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	if id == "" {
		WriteError(
			w,
			ErrorParams{Code: http.StatusBadRequest, ErrCode: "invalid_path", Err: errors.New("secret id is required")},
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
			ErrorParams{Code: http.StatusNotFound, ErrCode: "secret_not_found", Err: errors.New("secret not found")},
		)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]bool{"deleted": true})
}
