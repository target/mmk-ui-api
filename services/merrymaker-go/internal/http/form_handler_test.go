package httpx

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testFormData is a simple struct for testing the generic form handler.
type testFormData struct {
	Name  string
	Value string
}

// mockFormService implements FormService for testing.
type mockFormService struct {
	createFunc func(ctx context.Context, req testFormData) (any, error)
	updateFunc func(ctx context.Context, id string, req testFormData) (any, error)
}

func (m *mockFormService) Create(ctx context.Context, req testFormData) (any, error) {
	if m.createFunc != nil {
		return m.createFunc(ctx, req)
	}
	return &testFormData{Name: req.Name, Value: req.Value}, nil
}

func (m *mockFormService) Update(ctx context.Context, id string, req testFormData) (any, error) {
	if m.updateFunc != nil {
		return m.updateFunc(ctx, id, req)
	}
	return &testFormData{Name: req.Name, Value: req.Value}, nil
}

// mockFormParser creates a parser function for testing.
func mockFormParser(data testFormData, errs map[string]string) FormParser[testFormData] {
	return func(r *http.Request) (testFormData, map[string]string) {
		return data, errs
	}
}

// mockFormRenderer creates a renderer function for testing.
func mockFormRenderer(_ *testing.T) FormRenderer {
	return func(w http.ResponseWriter, r *http.Request, data map[string]any) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("form rendered"))
	}
}

func TestHandleForm_CreateSuccess(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	service := &mockFormService{}
	parser := mockFormParser(testFormData{Name: "test", Value: "value"}, nil)
	renderer := mockFormRenderer(t)

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should redirect on success
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "/success", w.Header().Get("Hx-Redirect"))
}

func TestHandleForm_CreateWithValidationErrors(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	service := &mockFormService{}
	fieldErrors := map[string]string{"name": "Name is required."}
	parser := mockFormParser(testFormData{}, fieldErrors)

	var renderedData map[string]any
	renderer := func(w http.ResponseWriter, r *http.Request, data map[string]any) {
		renderedData = data
		w.WriteHeader(http.StatusOK)
	}

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should render form with errors
	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, renderedData)
	assert.Equal(t, FormModeCreate, renderedData["Mode"])
	assert.Contains(t, renderedData, "Errors")
	assert.Contains(t, renderedData, "Error")
	assert.Equal(t, errMsgFixBelow, renderedData["ErrorMessage"])
}

func TestHandleForm_UpdateSuccess(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test/123", nil)
	r.SetPathValue("id", "123")

	service := &mockFormService{}
	parser := mockFormParser(testFormData{Name: "updated", Value: "new-value"}, nil)
	renderer := mockFormRenderer(t)

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeEdit,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should redirect on success
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "/success", w.Header().Get("Hx-Redirect"))
}

func TestHandleForm_UpdateWithValidationErrors(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test/123", nil)
	r.SetPathValue("id", "123")

	service := &mockFormService{}
	fieldErrors := map[string]string{"value": "Value is required."}
	parser := mockFormParser(testFormData{Name: "test"}, fieldErrors)

	var renderedData map[string]any
	renderer := func(w http.ResponseWriter, r *http.Request, data map[string]any) {
		renderedData = data
		w.WriteHeader(http.StatusOK)
	}

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeEdit,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should render form with errors
	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, renderedData)
	assert.Equal(t, FormModeEdit, renderedData["Mode"])
	assert.Contains(t, renderedData, "Errors")
}

func TestHandleForm_UpdateMissingID(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	service := &mockFormService{}
	parser := mockFormParser(testFormData{Name: "test", Value: "value"}, nil)
	renderer := mockFormRenderer(t)

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeEdit,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should return 404
	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestHandleForm_CreateServiceError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	service := &mockFormService{
		createFunc: func(ctx context.Context, req testFormData) (any, error) {
			return nil, errors.New("service error")
		},
	}
	parser := mockFormParser(testFormData{Name: "test", Value: "value"}, nil)

	var renderedData map[string]any
	renderer := func(w http.ResponseWriter, r *http.Request, data map[string]any) {
		renderedData = data
		w.WriteHeader(http.StatusOK)
	}

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should render form with error
	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, renderedData)
	assert.Equal(t, true, renderedData["Error"])
	assert.Equal(t, "Unable to save. Please try again.", renderedData["ErrorMessage"])
}

func TestHandleForm_UniqueConstraintViolation(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	// Create a mock PostgreSQL unique constraint violation error
	pgErr := &pgconn.PgError{
		Code:           "23505",
		Message:        "duplicate key value violates unique constraint",
		ConstraintName: "secrets_name_key",
	}

	service := &mockFormService{
		createFunc: func(ctx context.Context, req testFormData) (any, error) {
			return nil, pgErr
		},
	}
	parser := mockFormParser(testFormData{Name: "duplicate", Value: "value"}, nil)

	var renderedData map[string]any
	renderer := func(w http.ResponseWriter, r *http.Request, data map[string]any) {
		renderedData = data
		w.WriteHeader(http.StatusOK)
	}

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should render form with field error
	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, renderedData)
	errors, ok := renderedData["Errors"].(map[string]string)
	require.True(t, ok)
	assert.Contains(t, errors["name"], "already exists")
}

func TestHandleForm_ForeignKeyViolation(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	// Create a mock PostgreSQL foreign key violation error
	pgErr := &pgconn.PgError{
		Code:           "23503",
		Message:        "violates foreign key constraint",
		ConstraintName: "fk_constraint",
	}

	service := &mockFormService{
		createFunc: func(ctx context.Context, req testFormData) (any, error) {
			return nil, pgErr
		},
	}
	parser := mockFormParser(testFormData{Name: "test", Value: "value"}, nil)

	var renderedData map[string]any
	renderer := func(w http.ResponseWriter, r *http.Request, data map[string]any) {
		renderedData = data
		w.WriteHeader(http.StatusOK)
	}

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should render form with general error
	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, renderedData)
	assert.Equal(t, true, renderedData["Error"])
	assert.Contains(t, renderedData["ErrorMessage"], "related data constraints")
}

func TestHandleForm_CustomIDGetter(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test?id=custom-id", nil)

	service := &mockFormService{}
	parser := mockFormParser(testFormData{Name: "test", Value: "value"}, nil)
	renderer := mockFormRenderer(t)

	customIDCalled := false
	customGetID := func(r *http.Request) string {
		customIDCalled = true
		return r.URL.Query().Get("id")
	}

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeEdit,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
		GetID:      customGetID,
	})

	// Should use custom ID getter and redirect on success
	assert.True(t, customIDCalled)
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "/success", w.Header().Get("Hx-Redirect"))
}

func TestHandleForm_ExtraData(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	service := &mockFormService{}
	fieldErrors := map[string]string{"name": "Name is required."}
	parser := mockFormParser(testFormData{}, fieldErrors)

	var renderedData map[string]any
	renderer := func(w http.ResponseWriter, r *http.Request, data map[string]any) {
		renderedData = data
		w.WriteHeader(http.StatusOK)
	}

	extraData := map[string]any{
		"TypeOptions": []string{"Type1", "Type2"},
		"CustomField": "custom-value",
	}

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
		ExtraData:  extraData,
	})

	// Should include extra data in rendered template
	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, renderedData)
	assert.Equal(t, []string{"Type1", "Type2"}, renderedData["TypeOptions"])
	assert.Equal(t, "custom-value", renderedData["CustomField"])
}

func TestHandleForm_HTMXRedirect(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)
	r.Header.Set("Hx-Request", "true")

	service := &mockFormService{}
	parser := mockFormParser(testFormData{Name: "test", Value: "value"}, nil)
	renderer := mockFormRenderer(t)

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should set HTMX redirect header
	assert.Equal(t, http.StatusNoContent, w.Code)
	assert.Equal(t, "/success", w.Header().Get("Hx-Redirect"))
}

func TestHandleForm_UnknownDBError(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	// Create a mock PostgreSQL error with unknown code
	pgErr := &pgconn.PgError{
		Code:    "99999",
		Message: "unknown database error",
	}

	service := &mockFormService{
		createFunc: func(ctx context.Context, req testFormData) (any, error) {
			return nil, pgErr
		},
	}
	parser := mockFormParser(testFormData{Name: "test", Value: "value"}, nil)

	var renderedData map[string]any
	renderer := func(w http.ResponseWriter, r *http.Request, data map[string]any) {
		renderedData = data
		w.WriteHeader(http.StatusOK)
	}

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should render form with generic database error
	assert.Equal(t, http.StatusOK, w.Code)
	require.NotNil(t, renderedData)
	assert.Equal(t, true, renderedData["Error"])
	assert.Contains(t, renderedData["ErrorMessage"], "database error")
}

func TestHandleForm_GuardRails_MissingParser(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	service := &mockFormService{}
	renderer := mockFormRenderer(t)

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     nil, // Missing parser
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should return 500 error
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "misconfigured form handler")
}

func TestHandleForm_GuardRails_MissingService(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	parser := mockFormParser(testFormData{Name: "test", Value: "value"}, nil)
	renderer := mockFormRenderer(t)

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parser,
		Service:    nil, // Missing service
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should return 500 error
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "misconfigured form handler")
}

func TestHandleForm_GuardRails_MissingRenderer(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	service := &mockFormService{}
	parser := mockFormParser(testFormData{Name: "test", Value: "value"}, nil)

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parser,
		Service:    service,
		Renderer:   nil, // Missing renderer
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should return 500 error
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "misconfigured form handler")
}

func TestHandleForm_GuardRails_InvalidMode(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	service := &mockFormService{}
	parser := mockFormParser(testFormData{Name: "test", Value: "value"}, nil)
	renderer := mockFormRenderer(t)

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormMode("invalid"), // Invalid mode
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should return 400 error
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Contains(t, w.Body.String(), "invalid form mode")
}

func TestHandleForm_ContextCanceled(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately
	r := httptest.NewRequest(http.MethodPost, "/test", nil).WithContext(ctx)

	service := &mockFormService{
		createFunc: func(ctx context.Context, req testFormData) (any, error) {
			return nil, context.Canceled
		},
	}
	parser := mockFormParser(testFormData{Name: "test", Value: "value"}, nil)
	renderer := mockFormRenderer(t)

	HandleForm(FormHandlerOpts[testFormData]{
		W:          w,
		R:          r,
		Mode:       FormModeCreate,
		Parser:     parser,
		Service:    service,
		Renderer:   renderer,
		SuccessURL: "/success",
		PageMeta:   PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
	})

	// Should return 408 error
	assert.Equal(t, http.StatusRequestTimeout, w.Code)
	assert.Contains(t, w.Body.String(), "request canceled")
}

func TestHandleForm_ErrorStatus(t *testing.T) {
	t.Parallel()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodPost, "/test", nil)

	service := &mockFormService{}
	fieldErrors := map[string]string{"name": "Name is required."}
	parser := mockFormParser(testFormData{}, fieldErrors)

	var renderedData map[string]any
	renderer := func(w http.ResponseWriter, r *http.Request, data map[string]any) {
		renderedData = data
	}

	HandleForm(FormHandlerOpts[testFormData]{
		W:           w,
		R:           r,
		Mode:        FormModeCreate,
		Parser:      parser,
		Service:     service,
		Renderer:    renderer,
		SuccessURL:  "/success",
		PageMeta:    PageMeta{Title: "Test", PageTitle: "Test", CurrentPage: "test"},
		ErrorStatus: http.StatusBadRequest, // Set error status
	})

	// Should set 400 status for validation errors
	assert.Equal(t, http.StatusBadRequest, w.Code)
	require.NotNil(t, renderedData)
	assert.Contains(t, renderedData, "Errors")
}
