package httpx

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// ErrorRenderer is a function that renders an error template with the given data.
// This allows the error renderer to work with different rendering strategies.
type ErrorRenderer func(w http.ResponseWriter, r *http.Request, data any)

// ErrorOpts contains all options needed to render an error response.
// This struct is used to maintain the ≤3 parameters constraint while providing
// flexibility for different error scenarios.
type ErrorOpts struct {
	// W is the HTTP response writer
	W http.ResponseWriter
	// R is the HTTP request
	R *http.Request
	// Err is the error that occurred (optional, can be nil if only field errors)
	Err error
	// FieldErrors contains field-level validation errors (field name → error message)
	FieldErrors map[string]string
	// Renderer is the function to render the error template
	// This is typically h.renderDashboardPage or a similar function
	Renderer ErrorRenderer
	// PageMeta contains page metadata (title, current page, etc.)
	PageMeta PageMeta
	// Data contains additional template data to pass to the renderer
	// This is useful for preserving form data, dropdown options, etc.
	Data map[string]any
	// StatusCode is the HTTP status code to set (optional, defaults to 200 for HTMX compatibility)
	StatusCode int
	// ShowToast triggers a toast notification with the error message (optional)
	// When true, sends an HX-Trigger header with showToast event
	ShowToast bool
}

// DetermineErrorStatus determines the appropriate HTTP status code for an error.
// Returns http.StatusConflict (409) for foreign key violations, 0 (default) otherwise.
// A status of 0 means the caller should use the default behavior (typically 200 for HTMX).
//
// Usage:
//
//	status := DetermineErrorStatus(err)
//	RenderError(ErrorOpts{
//	    W: w, R: r, Err: err,
//	    StatusCode: status, // 409 for FK violations, 0 for others
//	    ...
//	})
func DetermineErrorStatus(err error) int {
	if err == nil {
		return 0
	}

	// Check if this is a foreign key violation
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgerrcode.ForeignKeyViolation {
		return http.StatusConflict
	}

	// For all other errors, return 0 to use default behavior
	// (typically 200 for HTMX partial updates, or 500 for server errors)
	return 0
}

// RenderError renders an error response using consistent error handling patterns.
// It supports field-level validation errors, general error messages, and database-specific errors.
//
// The function automatically detects and maps database errors (unique constraints, foreign keys)
// to user-friendly messages. It integrates with the template builder from Phase 1 and supports
// HTMX partial updates.
//
// Usage examples:
//
//	// Validation errors
//	RenderError(ErrorOpts{
//	    W: w, R: r,
//	    FieldErrors: map[string]string{"name": "Name is required."},
//	    Renderer: h.renderDashboardPage,
//	    PageMeta: PageMeta{Title: "Create Secret", CurrentPage: "secrets"},
//	})
//
//	// Database error
//	RenderError(ErrorOpts{
//	    W: w, R: r,
//	    Err: err, // Will be detected as unique constraint or foreign key violation
//	    Renderer: h.renderDashboardPage,
//	    PageMeta: PageMeta{Title: "Secrets", CurrentPage: "secrets"},
//	})
//
//	// General error with additional data
//	RenderError(ErrorOpts{
//	    W: w, R: r,
//	    Err: errors.New("something went wrong"),
//	    Renderer: h.renderDashboardPage,
//	    PageMeta: PageMeta{Title: "Edit Site", CurrentPage: "sites"},
//	    Data: map[string]any{"Mode": "edit", "SiteID": id},
//	})
func RenderError(opts ErrorOpts) {
	// Guard: ensure renderer is provided
	if opts.Renderer == nil {
		http.Error(opts.W, "misconfigured error renderer", http.StatusInternalServerError)
		return
	}

	// Build template data using Phase 1 template builder
	builder := NewTemplateData(opts.R, opts.PageMeta)

	// Process the error if present (this may add field errors)
	generalError := processError(opts.Err, &opts.FieldErrors)

	// Add field errors (including any added by processError)
	if len(opts.FieldErrors) > 0 {
		builder.WithFieldErrors(opts.FieldErrors)
	}

	// Add general error message
	if generalError != "" {
		builder.WithError(generalError)
	} else if len(opts.FieldErrors) > 0 {
		// If we have field errors but no general error, use default message
		builder.WithError(errMsgFixBelow)
	}

	// Add any additional data
	if opts.Data != nil {
		for k, v := range opts.Data {
			builder.With(k, v)
		}
	}

	// Trigger toast notification if requested
	if opts.ShowToast && generalError != "" {
		triggerToast(opts.W, generalError, "error")
	}

	// Set HTTP status code if specified
	if opts.StatusCode != 0 {
		opts.W.WriteHeader(opts.StatusCode)
	}

	// Render using the provided renderer
	opts.Renderer(opts.W, opts.R, builder.Build())
}

// processError processes an error and returns a user-friendly error message.
// It also updates fieldErrors if the error can be mapped to a specific field.
// Returns empty string if err is nil.
func processError(err error, fieldErrors *map[string]string) string {
	if err == nil {
		return ""
	}

	// Distinguish between timeout and cancellation for better UX
	if errors.Is(err, context.DeadlineExceeded) {
		return "Request timed out. Please try again."
	}
	if errors.Is(err, context.Canceled) {
		return "Request was canceled."
	}

	// Check for PostgreSQL errors
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return processDBError(pgErr, fieldErrors)
	}

	// Generic error
	return "An error occurred. Please try again."
}

// processDBError processes PostgreSQL-specific errors and returns user-friendly messages.
// It updates fieldErrors for unique constraint violations, NotNull, and Check violations when possible.
func processDBError(pgErr *pgconn.PgError, fieldErrors *map[string]string) string {
	switch pgErr.Code {
	case pgerrcode.UniqueViolation:
		return handleUniqueViolation(pgErr, fieldErrors)
	case pgerrcode.ForeignKeyViolation:
		return handleForeignKeyViolation(pgErr)
	case pgerrcode.CheckViolation:
		return handleCheckViolation(pgErr, fieldErrors)
	case pgerrcode.NotNullViolation:
		return handleNotNullViolation(pgErr, fieldErrors)
	default:
		return "A database error occurred. Please try again."
	}
}

// handleUniqueViolation handles unique constraint violations.
// It attempts to use ColumnName metadata first, then falls back to inferring
// the field name from the constraint name.
func handleUniqueViolation(pgErr *pgconn.PgError, fieldErrors *map[string]string) string {
	var field string

	// Prefer ColumnName metadata when available (more reliable)
	if pgErr.ColumnName != "" {
		field = pgErr.ColumnName
	} else {
		// Fallback: best-effort inference from constraint name
		// e.g., "secrets_name_key" → "name"
		field = inferFieldFromConstraint(pgErr.ConstraintName)
	}

	// Add field error if we have a field name
	if field != "" && fieldErrors != nil {
		if *fieldErrors == nil {
			*fieldErrors = make(map[string]string)
		}
		(*fieldErrors)[field] = "This value already exists. Please choose a different one."
		return errMsgFixBelow
	}

	// Fallback to general error if we can't determine the field
	return "This value already exists. Please choose a different one."
}

// handleForeignKeyViolation handles foreign key constraint violations.
// It provides context-aware messages using PgError metadata when available,
// falling back to constraint name heuristics.
func handleForeignKeyViolation(pgErr *pgconn.PgError) string {
	// Prefer structured metadata when available (more robust than constraint name parsing)
	if pgErr.TableName != "" {
		// TableName contains the referencing table (the one that has the FK)
		// This is more reliable than parsing constraint names
		return "Cannot complete operation because this item is in use by " + pgErr.TableName + "."
	}

	// Fallback to constraint-based heuristics for older PostgreSQL versions
	// or when TableName is not populated
	constraintName := strings.ToLower(pgErr.ConstraintName)

	// Check for secret first (before source, since "sources_secret_id_fkey" contains both)
	if strings.Contains(constraintName, "secret") {
		return "Cannot delete secret because it is in use by a Source or HTTP Alert Sink."
	}

	// Check for alert/sink before site (since "sites_alert_sink_id_fkey" contains both)
	if strings.Contains(constraintName, "alert") || strings.Contains(constraintName, "sink") {
		return "Cannot delete because it is in use by an HTTP Alert Sink."
	}

	// Common patterns for delete operations
	if strings.Contains(constraintName, "source") {
		return "Cannot delete because it is in use by a Source."
	}
	if strings.Contains(constraintName, "site") {
		return "Cannot delete because it is in use by a Site."
	}

	// Generic foreign key violation message
	return "Cannot complete operation because this item is in use."
}

// handleNotNullViolation handles NOT NULL constraint violations.
// It attempts to add field-level errors when ColumnName is available.
func handleNotNullViolation(pgErr *pgconn.PgError, fieldErrors *map[string]string) string {
	// Use ColumnName metadata when available for field-level error
	if pgErr.ColumnName != "" && fieldErrors != nil {
		if *fieldErrors == nil {
			*fieldErrors = make(map[string]string)
		}
		(*fieldErrors)[pgErr.ColumnName] = "This field is required."
		return errMsgFixBelow
	}

	// Fallback to general error if no column name
	return "Required field is missing. Please check your input."
}

// handleCheckViolation handles CHECK constraint violations.
// It attempts to add field-level errors when ColumnName is available.
func handleCheckViolation(pgErr *pgconn.PgError, fieldErrors *map[string]string) string {
	// Use ColumnName metadata when available for field-level error
	if pgErr.ColumnName != "" && fieldErrors != nil {
		if *fieldErrors == nil {
			*fieldErrors = make(map[string]string)
		}
		(*fieldErrors)[pgErr.ColumnName] = "This field has an invalid value."
		return errMsgFixBelow
	}

	// Fallback to general error if no column name
	return "Invalid data. Please check your input."
}

// inferFieldFromConstraint attempts to infer the field name from a constraint name.
// e.g., "secrets_name_key" → "name"
// e.g., "sites_name_unique" → "name"
// Returns empty string if inference fails or is ambiguous.
//
// Edge cases handled:
// - Multi-column constraints (e.g., "table_field1_field2_key") → "" (ambiguous).
// - Expression indexes (e.g., "table_lower_email_key") → "" (not a direct field).
func inferFieldFromConstraint(constraintName string) string {
	if constraintName == "" {
		return ""
	}

	parts := strings.Split(constraintName, "_")
	// Constraint names typically follow patterns like:
	// - "table_field_key" (unique) → 3 parts
	// - "table_field_unique" → 3 parts
	// - "table_field_idx" → 3 parts

	// Multi-column or complex constraints have more parts
	// e.g., "table_field1_field2_key" → 4+ parts
	// Avoid returning misleading field names for these cases
	if len(parts) > 3 {
		return "" // Ambiguous: could be multi-column or expression index
	}

	if len(parts) == 3 {
		fieldCandidate := parts[1] // The middle segment

		// Check if this looks like a function name (common in expression indexes)
		// e.g., "table_lower_key" where "lower" is a function, not a field
		if isFunctionName(fieldCandidate) {
			return "" // Expression index, not a direct field
		}

		return fieldCandidate
	}

	return "" // Not enough parts to infer
}

// isFunctionName checks if a string looks like a common SQL function name
// used in expression indexes (e.g., lower, upper, trim, etc.)
func isFunctionName(s string) bool {
	commonFunctions := []string{
		"lower", "upper", "trim", "ltrim", "rtrim",
		"md5", "sha1", "sha256", "encode", "decode",
	}
	s = strings.ToLower(s)
	// Check if s is in the list of common functions
	for _, fn := range commonFunctions {
		if s == fn {
			return true
		}
	}
	return false
}
