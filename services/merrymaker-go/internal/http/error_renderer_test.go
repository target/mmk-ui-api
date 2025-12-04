package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgerrcode"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockRenderer captures the data passed to it for testing.
type mockRenderer struct {
	called bool
	w      http.ResponseWriter
	r      *http.Request
	data   map[string]any
}

func (m *mockRenderer) render(w http.ResponseWriter, r *http.Request, data any) {
	m.called = true
	m.w = w
	m.r = r
	if typed, ok := data.(map[string]any); ok {
		m.data = typed
	} else {
		m.data = nil
	}
}

func TestRenderError_FieldErrors(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	fieldErrors := map[string]string{
		"name":  "Name is required.",
		"email": "Invalid email address.",
	}

	RenderError(ErrorOpts{
		W:           w,
		R:           r,
		FieldErrors: fieldErrors,
		Renderer:    mock.render,
		PageMeta: PageMeta{
			Title:       "Test Page",
			PageTitle:   "Test",
			CurrentPage: "test",
		},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.NotNil(t, mock.data, "data should not be nil")

	// Check that field errors are present
	errors, ok := mock.data["Errors"].(map[string]string)
	require.True(t, ok, "Errors should be a map[string]string")
	assert.Equal(t, "Name is required.", errors["name"])
	assert.Equal(t, "Invalid email address.", errors["email"])

	// Check that general error message is set
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, errMsgFixBelow, mock.data["ErrorMessage"])
}

func TestRenderError_GeneralError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      errors.New("something went wrong"),
		Renderer: mock.render,
		PageMeta: PageMeta{
			Title:       "Test Page",
			PageTitle:   "Test",
			CurrentPage: "test",
		},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, "An error occurred. Please try again.", mock.data["ErrorMessage"])
}

func TestRenderError_UniqueViolation(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code:           pgerrcode.UniqueViolation,
		ConstraintName: "secrets_name_key",
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{
			Title:       "Test Page",
			PageTitle:   "Test",
			CurrentPage: "test",
		},
	})

	assert.True(t, mock.called, "renderer should be called")

	// Check that field error is added
	errors, ok := mock.data["Errors"].(map[string]string)
	require.True(t, ok, "Errors should be a map[string]string")
	assert.Equal(t, "This value already exists. Please choose a different one.", errors["name"])

	// Check that general error message is set
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, errMsgFixBelow, mock.data["ErrorMessage"])
}

func TestRenderError_ForeignKeyViolation_Secret(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code:           pgerrcode.ForeignKeyViolation,
		ConstraintName: "sources_secret_id_fkey",
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{
			Title:       "Test Page",
			PageTitle:   "Test",
			CurrentPage: "test",
		},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(
		t,
		"Cannot delete secret because it is in use by a Source or HTTP Alert Sink.",
		mock.data["ErrorMessage"],
	)
}

func TestRenderError_ForeignKeyViolation_Site(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code:           pgerrcode.ForeignKeyViolation,
		ConstraintName: "jobs_site_id_fkey",
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{
			Title:       "Test Page",
			PageTitle:   "Test",
			CurrentPage: "test",
		},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, "Cannot delete because it is in use by a Site.", mock.data["ErrorMessage"])
}

func TestRenderError_ContextCanceled(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      context.Canceled,
		Renderer: mock.render,
		PageMeta: PageMeta{
			Title:       "Test Page",
			PageTitle:   "Test",
			CurrentPage: "test",
		},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	// Updated: now distinguishes between canceled and timeout
	assert.Equal(t, "Request was canceled.", mock.data["ErrorMessage"])
}

func TestRenderError_WithAdditionalData(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	RenderError(ErrorOpts{
		W:          w,
		R:          r,
		Err:        errors.New("test error"),
		Renderer:   mock.render,
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
		Data:       map[string]any{"Mode": "edit", "ItemID": "123"},
		StatusCode: http.StatusBadRequest,
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.Equal(t, "edit", mock.data["Mode"])
	assert.Equal(t, "123", mock.data["ItemID"])
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestRenderError_NoRenderer(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      errors.New("test error"),
		Renderer: nil, // No renderer provided
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "misconfigured error renderer")
}

func TestInferFieldFromConstraint(t *testing.T) {
	tests := []struct {
		name           string
		constraintName string
		expected       string
	}{
		{
			name:           "standard unique constraint",
			constraintName: "secrets_name_key",
			expected:       "name",
		},
		{
			name:           "unique constraint with unique suffix",
			constraintName: "sites_name_unique",
			expected:       "name",
		},
		{
			name:           "index constraint",
			constraintName: "users_email_idx",
			expected:       "email",
		},
		{
			name:           "empty constraint name",
			constraintName: "",
			expected:       "",
		},
		{
			name:           "single part constraint",
			constraintName: "name",
			expected:       "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := inferFieldFromConstraint(tt.constraintName)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestHandleForeignKeyViolation(t *testing.T) {
	tests := []struct {
		name           string
		constraintName string
		expectedMsg    string
	}{
		{
			name:           "source constraint",
			constraintName: "sites_source_id_fkey",
			expectedMsg:    "Cannot delete because it is in use by a Source.",
		},
		{
			name:           "site constraint",
			constraintName: "jobs_site_id_fkey",
			expectedMsg:    "Cannot delete because it is in use by a Site.",
		},
		{
			name:           "alert sink constraint",
			constraintName: "sites_alert_sink_id_fkey",
			expectedMsg:    "Cannot delete because it is in use by an HTTP Alert Sink.",
		},
		{
			name:           "secret constraint",
			constraintName: "sources_secret_id_fkey",
			expectedMsg:    "Cannot delete secret because it is in use by a Source or HTTP Alert Sink.",
		},
		{
			name:           "generic constraint",
			constraintName: "unknown_fkey",
			expectedMsg:    "Cannot complete operation because this item is in use.",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pgErr := &pgconn.PgError{
				Code:           pgerrcode.ForeignKeyViolation,
				ConstraintName: tt.constraintName,
			}
			result := handleForeignKeyViolation(pgErr)
			assert.Equal(t, tt.expectedMsg, result)
		})
	}
}

func TestRenderError_CheckViolation(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code: pgerrcode.CheckViolation,
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, "Invalid data. Please check your input.", mock.data["ErrorMessage"])
}

func TestRenderError_NotNullViolation(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code: pgerrcode.NotNullViolation,
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, "Required field is missing. Please check your input.", mock.data["ErrorMessage"])
}

func TestRenderError_UniqueViolation_NoConstraintName(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code:           pgerrcode.UniqueViolation,
		ConstraintName: "", // No constraint name
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	// Should fall back to general error message
	assert.Equal(t, "This value already exists. Please choose a different one.", mock.data["ErrorMessage"])
}

func TestRenderError_UnknownDBError(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code: "99999", // Unknown error code
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, "A database error occurred. Please try again.", mock.data["ErrorMessage"])
}

// --- Tests for new improvements ---

func TestProcessError_ContextDeadlineExceeded(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      context.DeadlineExceeded,
		Renderer: mock.render,
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, "Request timed out. Please try again.", mock.data["ErrorMessage"])
}

func TestProcessError_ContextCanceled(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      context.Canceled,
		Renderer: mock.render,
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, "Request was canceled.", mock.data["ErrorMessage"])
}

func TestHandleNotNullViolation_WithColumnName(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code:       pgerrcode.NotNullViolation,
		ColumnName: "email",
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, errMsgFixBelow, mock.data["ErrorMessage"])

	// Check field error was added
	errors, ok := mock.data["Errors"].(map[string]string)
	require.True(t, ok, "Errors should be a map[string]string")
	assert.Equal(t, "This field is required.", errors["email"])
}

func TestHandleNotNullViolation_WithoutColumnName(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code:       pgerrcode.NotNullViolation,
		ColumnName: "", // No column name
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, "Required field is missing. Please check your input.", mock.data["ErrorMessage"])
}

func TestHandleCheckViolation_WithColumnName(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code:       pgerrcode.CheckViolation,
		ColumnName: "age",
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, errMsgFixBelow, mock.data["ErrorMessage"])

	// Check field error was added
	errors, ok := mock.data["Errors"].(map[string]string)
	require.True(t, ok, "Errors should be a map[string]string")
	assert.Equal(t, "This field has an invalid value.", errors["age"])
}

func TestHandleCheckViolation_WithoutColumnName(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code:       pgerrcode.CheckViolation,
		ColumnName: "", // No column name
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, "Invalid data. Please check your input.", mock.data["ErrorMessage"])
}

func TestHandleForeignKeyViolation_WithTableName(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:           pgerrcode.ForeignKeyViolation,
		TableName:      "jobs",
		ConstraintName: "jobs_site_id_fkey",
	}

	result := handleForeignKeyViolation(pgErr)
	assert.Equal(t, "Cannot complete operation because this item is in use by jobs.", result)
}

func TestHandleUniqueViolation_WithColumnName(t *testing.T) {
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/test", nil)
	mock := &mockRenderer{}

	pgErr := &pgconn.PgError{
		Code:           pgerrcode.UniqueViolation,
		ColumnName:     "email", // Prefer ColumnName over constraint inference
		ConstraintName: "users_email_key",
	}

	RenderError(ErrorOpts{
		W:        w,
		R:        r,
		Err:      pgErr,
		Renderer: mock.render,
		PageMeta: PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	assert.True(t, mock.called, "renderer should be called")
	assert.True(t, mock.data["Error"].(bool))
	assert.Equal(t, errMsgFixBelow, mock.data["ErrorMessage"])

	// Check field error was added using ColumnName
	errors, ok := mock.data["Errors"].(map[string]string)
	require.True(t, ok, "Errors should be a map[string]string")
	assert.Equal(t, "This value already exists. Please choose a different one.", errors["email"])
}

func TestInferFieldFromConstraint_MultiColumn(t *testing.T) {
	// Multi-column constraint should return empty string (ambiguous)
	result := inferFieldFromConstraint("users_email_username_key")
	assert.Empty(t, result, "multi-column constraint should return empty string")
}

func TestInferFieldFromConstraint_ExpressionIndex(t *testing.T) {
	// Expression index with function should return empty string
	result := inferFieldFromConstraint("users_lower_key")
	assert.Empty(t, result, "expression index should return empty string")

	result = inferFieldFromConstraint("users_upper_key")
	assert.Empty(t, result, "expression index should return empty string")
}

func TestInferFieldFromConstraint_ValidSingleColumn(t *testing.T) {
	// Valid single-column constraint
	result := inferFieldFromConstraint("secrets_name_key")
	assert.Equal(t, "name", result)

	result = inferFieldFromConstraint("sites_domain_unique")
	assert.Equal(t, "domain", result)
}

func TestIsFunctionName(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected bool
	}{
		{"lower", "lower", true},
		{"LOWER", "LOWER", true},
		{"upper", "upper", true},
		{"trim", "trim", true},
		{"md5", "md5", true},
		{"not_a_function", "email", false},
		{"not_a_function", "username", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := isFunctionName(tt.input)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestDetermineErrorStatus_ForeignKeyViolation(t *testing.T) {
	pgErr := &pgconn.PgError{
		Code:           pgerrcode.ForeignKeyViolation,
		ConstraintName: "sources_secret_id_fkey",
	}

	status := DetermineErrorStatus(pgErr)
	assert.Equal(t, http.StatusConflict, status)
}

func TestDetermineErrorStatus_OtherError(t *testing.T) {
	// Generic error should return 0 (use default)
	status := DetermineErrorStatus(errors.New("generic error"))
	assert.Equal(t, 0, status)

	// Unique violation should return 0 (not a conflict in the FK sense)
	pgErr := &pgconn.PgError{
		Code:           pgerrcode.UniqueViolation,
		ConstraintName: "secrets_name_key",
	}
	status = DetermineErrorStatus(pgErr)
	assert.Equal(t, 0, status)
}

func TestDetermineErrorStatus_NilError(t *testing.T) {
	status := DetermineErrorStatus(nil)
	assert.Equal(t, 0, status)
}
