package httpx

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
)

// FormParser parses form data from an HTTP request and returns the parsed data
// along with any field-level validation errors.
type FormParser[T any] func(r *http.Request) (T, map[string]string)

// FormService defines the interface for services that support Create and Update operations.
// Services must implement both methods to be used with the generic form handler.
type FormService[T any] interface {
	Create(ctx context.Context, req T) (any, error)
	Update(ctx context.Context, id string, req T) (any, error)
}

// FormRenderer is a function that renders the form template with the given data.
// This allows the form handler to work with different rendering strategies.
type FormRenderer func(w http.ResponseWriter, r *http.Request, data map[string]any)

// ErrorHandler is a function that handles service errors and returns field errors and a general error message.
// Return nil for both if the error should be handled by the default handler.
type ErrorHandler func(err error) (fieldErrors map[string]string, generalError string)

// FormHandlerOpts contains all options needed to handle a form submission.
// It uses a single struct parameter to maintain the ≤3 parameters constraint.
type FormHandlerOpts[T any] struct {
	W        http.ResponseWriter // Response writer
	R        *http.Request       // Request
	Mode     FormMode            // "create" or "edit"
	Parser   FormParser[T]       // Function to parse form data
	Service  FormService[T]      // Service to call for Create/Update
	Renderer FormRenderer        // Function to render form with data
	// Success redirect URL
	SuccessURL string
	// Page metadata for rendering
	PageMeta PageMeta
	// Optional: additional data to pass to template on error
	ExtraData map[string]any
	// Optional: function to extract ID from request (defaults to r.PathValue("id"))
	GetID func(r *http.Request) string
	// Optional: custom error handler for domain-specific errors
	HandleError ErrorHandler
	// Optional: HTTP status code to set on validation errors (defaults to 200 for HTMX compatibility)
	ErrorStatus int
}

// HandleForm is a generic form handler that processes Create and Update workflows.
// It handles form parsing, validation, service calls, error handling, and redirects.
//
// Usage example:
//
//	HandleForm(FormHandlerOpts[types.CreateSecretRequest]{
//	    W: w, R: r, Mode: FormModeCreate,
//	    Parser: parseSecretForm,
//	    Service: h.SecretSvc,
//	    Renderer: h.renderSecretForm,
//	    SuccessURL: "/secrets",
//	    PageMeta: PageMeta{Title: "New Secret", PageTitle: "New Secret", CurrentPage: PageSecretForm},
//	})
func HandleForm[T any](opts FormHandlerOpts[T]) {
	// Guard rails: validate required options
	if !validateFormOptions(opts) {
		return
	}

	// For edit mode, check ID first before parsing
	id, ok := checkFormID(opts)
	if !ok {
		return
	}

	// Parse form data and get validation errors
	data, fieldErrors := opts.Parser(opts.R)

	// If validation errors exist, re-render form with errors
	if len(fieldErrors) > 0 {
		opts.renderFormError(fieldErrors, "", data)
		return
	}

	// Execute create or update operation
	err := executeFormOperation(opts, id, data)
	// Handle service errors
	if err != nil {
		handleFormServiceError(opts, err, data)
		return
	}

	// Success: redirect using HTMX helper
	HTMX(opts.W).Redirect(opts.SuccessURL)
}

// validateFormOptions validates required options and mode.
func validateFormOptions[T any](opts FormHandlerOpts[T]) bool {
	if opts.Parser == nil || opts.Service == nil || opts.Renderer == nil {
		http.Error(opts.W, "misconfigured form handler", http.StatusInternalServerError)
		return false
	}

	switch opts.Mode {
	case FormModeEdit, FormModeCreate:
		return true
	default:
		http.Error(opts.W, "invalid form mode", http.StatusBadRequest)
		return false
	}
}

// checkFormID checks and returns the ID for edit mode. Returns empty string and true for create mode.
func checkFormID[T any](opts FormHandlerOpts[T]) (string, bool) {
	if opts.Mode != FormModeEdit {
		return "", true
	}

	id := getFormID(opts)
	if id == "" {
		http.NotFound(opts.W, opts.R)
		return "", false
	}
	return id, true
}

// executeFormOperation executes the create or update operation based on mode.
func executeFormOperation[T any](opts FormHandlerOpts[T], id string, data T) error {
	if opts.Mode == FormModeEdit {
		_, err := opts.Service.Update(opts.R.Context(), id, data)
		return err
	}
	_, err := opts.Service.Create(opts.R.Context(), data)
	return err
}

// getFormID extracts the ID from the request, using custom getter if provided.
func getFormID[T any](opts FormHandlerOpts[T]) string {
	if opts.GetID != nil {
		return opts.GetID(opts.R)
	}
	return opts.R.PathValue("id")
}

// handleFormServiceError handles errors from service Create/Update calls.
func handleFormServiceError[T any](opts FormHandlerOpts[T], err error, data T) {
	// Special-case context cancellation/timeouts to avoid noisy UX
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		http.Error(opts.W, "request canceled", http.StatusRequestTimeout)
		return
	}

	// Try custom error handler first if provided
	if opts.HandleError != nil {
		fieldErrors, generalError := opts.HandleError(err)
		if fieldErrors != nil || generalError != "" {
			opts.renderFormError(fieldErrors, generalError, data)
			return
		}
	}

	// Check for database errors
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		handleDBError(opts, pgErr, data)
		return
	}

	// Generic error
	opts.renderFormError(nil, "Unable to save. Please try again.", data)
}

// handleDBError handles PostgreSQL-specific errors.
func handleDBError[T any](opts FormHandlerOpts[T], pgErr *pgconn.PgError, data T) {
	var fieldErrors map[string]string
	var generalError string

	switch pgErr.Code {
	case pgerrcode.UniqueViolation:
		// Best-effort inference of field name from constraint name
		// e.g., "secrets_name_key" → "name"
		field := "name" // default fallback
		if pgErr.ConstraintName != "" {
			parts := strings.Split(pgErr.ConstraintName, "_")
			// Take the second-to-last segment as the field name
			// e.g., "secrets_name_key" → ["secrets", "name", "key"] → "name"
			if len(parts) >= 2 {
				field = parts[len(parts)-2]
			}
		}
		fieldErrors = map[string]string{
			field: "This value already exists. Please choose a different one.",
		}
	case pgerrcode.ForeignKeyViolation:
		generalError = "Cannot complete operation due to related data constraints."
	default:
		generalError = "A database error occurred. Please try again."
	}

	opts.renderFormError(fieldErrors, generalError, data)
}

// renderFormError renders the form with errors and preserves form data.
func (fh FormHandlerOpts[T]) renderFormError(fieldErrors map[string]string, generalError string, data T) {
	// Debug aid: optionally log form errors for troubleshooting tests
	// Set MM_DEBUG_FORMS=1 to enable
	if os.Getenv("MM_DEBUG_FORMS") == "1" {
		fmt.Fprintf(
			os.Stderr,
			"FormError mode=%v id=%s fieldErrors=%v general=%q\n",
			fh.Mode,
			getFormID(fh),
			fieldErrors,
			generalError,
		)
	}

	// Set HTTP status code for validation errors if configured
	if fh.ErrorStatus != 0 && len(fieldErrors) > 0 {
		fh.W.WriteHeader(fh.ErrorStatus)
	}

	templateData := NewTemplateData(fh.R, fh.PageMeta)

	// Add field errors if present
	if len(fieldErrors) > 0 {
		templateData.WithFieldErrors(fieldErrors)
	}

	// Add general error if present
	if generalError != "" {
		templateData.WithError(generalError)
	} else if len(fieldErrors) > 0 {
		templateData.WithError(errMsgFixBelow)
	}

	// Add mode
	templateData.With("Mode", fh.Mode)

	// Add any extra data first (so FormData can override if needed)
	if fh.ExtraData != nil {
		for k, v := range fh.ExtraData {
			templateData.With(k, v)
		}
	}

	// Add form data - this allows templates to access the parsed form data
	// Templates can access individual fields or the whole struct
	templateData.With("FormData", data)

	// Render the form template
	renderFormTemplate(fh, templateData.Build())
}

// renderFormTemplate renders the form using the provided renderer function.
func renderFormTemplate[T any](opts FormHandlerOpts[T], data map[string]any) {
	opts.Renderer(opts.W, opts.R, data)
}
