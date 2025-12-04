//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"errors"
	"net/url"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// HTTP alert sink name constraints.
	minHTTPAlertSinkNameLen = 3
	maxHTTPAlertSinkNameLen = 512
	maxURILen               = 1024
)

// HTTPAlertSink represents an HTTP alert sink configuration in the system.
type HTTPAlertSink struct {
	ID          string    `json:"id"                     db:"id"`
	Name        string    `json:"name"                   db:"name"`
	URI         string    `json:"uri"                    db:"uri"`
	Method      string    `json:"method"                 db:"method"`
	Body        *string   `json:"body,omitempty"         db:"body"`
	QueryParams *string   `json:"query_params,omitempty" db:"query_params"`
	Headers     *string   `json:"headers,omitempty"      db:"headers"`
	OkStatus    int       `json:"ok_status"              db:"ok_status"`
	Retry       int       `json:"retry"                  db:"retry"`
	Secrets     []string  `json:"secrets"                db:"secrets"`
	CreatedAt   time.Time `json:"created_at"             db:"created_at"`
}

// CreateHTTPAlertSinkRequest represents a request to create a new HTTP alert sink.
type CreateHTTPAlertSinkRequest struct {
	Name        string   `json:"name"`
	URI         string   `json:"uri"`
	Method      string   `json:"method"`
	Body        *string  `json:"body,omitempty"`
	QueryParams *string  `json:"query_params,omitempty"`
	Headers     *string  `json:"headers,omitempty"`
	OkStatus    *int     `json:"ok_status,omitempty"`
	Retry       *int     `json:"retry,omitempty"`
	Secrets     []string `json:"secrets,omitempty"`
}

// UpdateHTTPAlertSinkRequest represents a request to update an existing HTTP alert sink.
type UpdateHTTPAlertSinkRequest struct {
	Name        *string  `json:"name,omitempty"`
	URI         *string  `json:"uri,omitempty"`
	Method      *string  `json:"method,omitempty"`
	Body        *string  `json:"body,omitempty"`
	QueryParams *string  `json:"query_params,omitempty"`
	Headers     *string  `json:"headers,omitempty"`
	OkStatus    *int     `json:"ok_status,omitempty"`
	Retry       *int     `json:"retry,omitempty"`
	Secrets     []string `json:"secrets,omitempty"`
}

// Normalize normalizes the CreateHTTPAlertSinkRequest fields.
func (r *CreateHTTPAlertSinkRequest) Normalize() {
	r.Name = strings.TrimSpace(r.Name)
	r.URI = strings.TrimSpace(r.URI)
	r.Method = strings.ToUpper(strings.TrimSpace(r.Method))
}

// Validate validates the CreateHTTPAlertSinkRequest fields.
func (r *CreateHTTPAlertSinkRequest) Validate() error {
	if err := validateHTTPAlertSinkName(r.Name); err != nil {
		return err
	}

	if err := validateHTTPAlertSinkURI(r.URI); err != nil {
		return err
	}

	if err := validateHTTPMethod(r.Method); err != nil {
		return err
	}

	if err := validateOkStatus(r.OkStatus); err != nil {
		return err
	}

	if err := validateRetry(r.Retry); err != nil {
		return err
	}

	return validateHTTPAlertSinkSecretsSlice(r.Secrets)
}

// Normalize normalizes the UpdateHTTPAlertSinkRequest fields.
func (r *UpdateHTTPAlertSinkRequest) Normalize() {
	if r.Name != nil {
		n := strings.TrimSpace(*r.Name)
		r.Name = &n
	}
	if r.URI != nil {
		u := strings.TrimSpace(*r.URI)
		r.URI = &u
	}
	if r.Method != nil {
		m := strings.ToUpper(strings.TrimSpace(*r.Method))
		r.Method = &m
	}
}

// Validate validates the UpdateHTTPAlertSinkRequest fields and ensures at least one field is being updated.
func (r *UpdateHTTPAlertSinkRequest) Validate() error {
	if !r.HasUpdates() {
		return errors.New("at least one field must be updated")
	}

	return r.validateFields()
}

// validateFields validates individual fields in the update request.
func (r *UpdateHTTPAlertSinkRequest) validateFields() error {
	if r.Name != nil {
		if err := validateHTTPAlertSinkName(*r.Name); err != nil {
			return err
		}
	}

	if r.URI != nil {
		if err := validateHTTPAlertSinkURI(*r.URI); err != nil {
			return err
		}
	}

	if r.Method != nil {
		if err := validateHTTPMethod(*r.Method); err != nil {
			return err
		}
	}

	if err := validateOkStatus(r.OkStatus); err != nil {
		return err
	}

	if err := validateRetry(r.Retry); err != nil {
		return err
	}

	if r.Secrets != nil {
		return validateHTTPAlertSinkSecretsSlice(r.Secrets)
	}

	return nil
}

// HasUpdates returns true if the UpdateHTTPAlertSinkRequest has any fields to update.
func (r *UpdateHTTPAlertSinkRequest) HasUpdates() bool {
	return r.Name != nil || r.URI != nil || r.Method != nil ||
		r.Body != nil || r.QueryParams != nil || r.Headers != nil ||
		r.OkStatus != nil || r.Retry != nil || r.Secrets != nil
}

// validateHTTPAlertSinkName validates the name field.
func validateHTTPAlertSinkName(name string) error {
	trimmed := strings.TrimSpace(name)
	if trimmed == "" {
		return errors.New("name is required and cannot be empty")
	}

	nameLen := utf8.RuneCountInString(trimmed)
	if nameLen < minHTTPAlertSinkNameLen {
		return errors.New("name must be at least 3 characters")
	}
	if nameLen > maxHTTPAlertSinkNameLen {
		return errors.New("name cannot exceed 512 characters")
	}

	return nil
}

// validateHTTPAlertSinkURI validates the URI field.
func validateHTTPAlertSinkURI(uri string) error {
	trimmed := strings.TrimSpace(uri)
	if trimmed == "" {
		return errors.New("uri is required and cannot be empty")
	}

	if utf8.RuneCountInString(trimmed) > maxURILen {
		return errors.New("uri cannot exceed 1024 characters")
	}

	// Validate that it's a valid URL with http/https scheme and host
	parsed, err := url.Parse(trimmed)
	if err != nil {
		return errors.New("uri must be a valid URL")
	}

	// Require http or https scheme
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("uri must use http or https scheme")
	}

	// Require a non-empty host
	if parsed.Host == "" {
		return errors.New("uri must have a valid host")
	}

	return nil
}

// validateHTTPMethod validates the HTTP method field.
func validateHTTPMethod(method string) error {
	trimmed := strings.TrimSpace(strings.ToUpper(method))
	if trimmed == "" {
		return errors.New("method is required and cannot be empty")
	}

	// Check against allowed HTTP methods
	switch trimmed {
	case "GET", "POST", "PUT", "PATCH", "DELETE":
		return nil
	default:
		return errors.New("method must be one of: GET, POST, PUT, PATCH, DELETE")
	}
}

// validateOkStatus validates the ok_status field.
func validateOkStatus(okStatus *int) error {
	if okStatus != nil && (*okStatus < 100 || *okStatus > 599) {
		return errors.New("ok_status must be between 100 and 599")
	}
	return nil
}

// validateRetry validates the retry field.
func validateRetry(retry *int) error {
	if retry != nil && *retry < 0 {
		return errors.New("retry must be non-negative")
	}
	return nil
}

// validateHTTPAlertSinkSecretsSlice validates that all entries in a secrets slice are non-empty and unique.
func validateHTTPAlertSinkSecretsSlice(secrets []string) error {
	seen := make(map[string]bool)

	for _, secret := range secrets {
		trimmed := strings.TrimSpace(secret)
		if trimmed == "" {
			return errors.New("secrets cannot contain empty or whitespace-only entries")
		}

		if seen[trimmed] {
			return errors.New("secrets cannot contain duplicate entries")
		}
		seen[trimmed] = true
	}
	return nil
}
