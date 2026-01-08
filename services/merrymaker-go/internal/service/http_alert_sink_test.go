package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"go.uber.org/mock/gomock"
)

type evalStub struct {
	validateErr error
	res         any
	evalErr     error
}

func (e evalStub) Validate(_ string) error               { return e.validateErr }
func (e evalStub) Evaluate(_ string, _ any) (any, error) { return e.res, e.evalErr }

func TestAlertSinkService_ResolveSecrets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)

	svc := NewAlertSinkService(AlertSinkServiceOptions{JobRepo: jobRepo, SecretRepo: secretRepo, Evaluator: evalStub{}})

	sink := model.HTTPAlertSink{
		ID:          "sink-1",
		Name:        "test",
		URI:         "https://example.com/path",
		Method:      "POST",
		Body:        ptr("token=__API_KEY__"),
		QueryParams: ptr("key=__API_KEY__&a=b"),
		Headers:     ptr("Authorization: Bearer __API_KEY__"),
		OkStatus:    200,
		Retry:       1,
		Secrets:     []string{"API_KEY"},
	}

	secretRepo.EXPECT().GetByName(gomock.Any(), "API_KEY").Return(&model.Secret{Name: "API_KEY", Value: "XYZ"}, nil)

	resolved, placeholders, err := svc.ResolveSecrets(context.Background(), sink)
	require.NoError(t, err)
	assert.Equal(t, "token=XYZ", *resolved.Body)
	assert.Equal(t, "key=XYZ&a=b", *resolved.QueryParams)
	assert.Equal(t, "Authorization: Bearer XYZ", *resolved.Headers)
	assert.Equal(t, map[string]string{"__API_KEY__": "XYZ"}, placeholders)
}

func TestAlertSinkService_ValidateSinkConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)

	// invalid jmespath
	svc := NewAlertSinkService(
		AlertSinkServiceOptions{
			JobRepo:    jobRepo,
			SecretRepo: secretRepo,
			Evaluator:  evalStub{validateErr: assert.AnError},
		},
	)
	sink := model.HTTPAlertSink{Method: "POST", Body: ptr("bad expr"), Secrets: []string{"S"}}
	// Since validation fails on JMESPath, secret resolution should not be invoked
	err := svc.ValidateSinkConfiguration(context.Background(), sink)
	require.Error(t, err)

	// valid
	svcOK := NewAlertSinkService(
		AlertSinkServiceOptions{JobRepo: jobRepo, SecretRepo: secretRepo, Evaluator: evalStub{}},
	)
	sinkOK := model.HTTPAlertSink{
		Method:  "POST",
		URI:     "https://example.com",
		Body:    ptr("items"),
		Secrets: []string{"S"},
	}
	secretRepo.EXPECT().GetByName(gomock.Any(), "S").Return(&model.Secret{Name: "S", Value: "v"}, nil)
	require.NoError(t, svcOK.ValidateSinkConfiguration(context.Background(), sinkOK))
}

func TestAlertSinkService_ProcessSinkConfiguration(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)

	svc := NewAlertSinkService(
		AlertSinkServiceOptions{
			JobRepo:    jobRepo,
			SecretRepo: secretRepo,
			Evaluator:  evalStub{res: map[string]any{"k": "v"}},
		},
	)

	sink := model.HTTPAlertSink{
		ID:          "sink-1",
		Name:        "test",
		URI:         "https://api.example.com/alert",
		Method:      "POST",
		Body:        ptr("some.expr"),
		QueryParams: ptr("token=__T__"),
		Headers:     ptr("X-API: __T__\nAccept: application/json"),
		OkStatus:    204,
		Retry:       2,
		Secrets:     []string{"T"},
	}
	secretRepo.EXPECT().GetByName(gomock.Any(), "T").Return(&model.Secret{Name: "T", Value: "abc"}, nil)

	payload := json.RawMessage(`{"foo":"bar"}`)
	prep, err := svc.ProcessSinkConfiguration(context.Background(), sink, payload)
	require.NoError(t, err)
	assert.Equal(t, "POST", prep.Method)
	assert.Equal(t, "https://api.example.com/alert?token=abc", prep.URL)
	assert.Equal(t, 204, prep.OkStatus)
	assert.Equal(t, "application/json", prep.Headers["Accept"])
	assert.Equal(t, "abc", prep.Headers["X-API"])
	assert.Equal(t, map[string]string{"__T__": "abc"}, prep.Secrets)
	// body derived from evaluator
	assert.JSONEq(t, `{"k":"v"}`, string(prep.Body))
}

func TestAlertSinkService_ProcessSinkConfiguration_DefaultContentType(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)

	svc := NewAlertSinkService(
		AlertSinkServiceOptions{
			JobRepo:    jobRepo,
			SecretRepo: secretRepo,
			Evaluator:  evalStub{res: map[string]any{"k": "v"}},
		},
	)

	// Headers do not include Content-Type; body will be derived
	sink := model.HTTPAlertSink{
		ID:          "sink-ct",
		Name:        "test-default-ct",
		URI:         "https://api.example.com/alert",
		Method:      "POST",
		Body:        ptr("some.expr"),
		QueryParams: ptr("a=b"),
		Headers:     ptr("X-Trace: abc123"),
		OkStatus:    200,
		Retry:       1,
	}

	payload := json.RawMessage(`{"foo":"bar"}`)
	prep, err := svc.ProcessSinkConfiguration(context.Background(), sink, payload)
	require.NoError(t, err)
	// Default Content-Type should be added when body is present and header missing
	assert.Equal(t, "application/json", prep.Headers["Content-Type"])
	// Existing headers remain
	assert.Equal(t, "abc123", prep.Headers["X-Trace"])
	// Body derived from evaluator
	assert.JSONEq(t, `{"k":"v"}`, string(prep.Body))
}

func TestAlertSinkService_ParseJSONHeaders(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)
	svc := NewAlertSinkService(AlertSinkServiceOptions{JobRepo: jobRepo, SecretRepo: secretRepo, Evaluator: evalStub{}})

	sink := model.HTTPAlertSink{
		ID:     "sink-json",
		Name:   "test-json",
		URI:    "https://api.example.com/alert",
		Method: "POST",
		// JSON headers with secret placeholder
		Headers: ptr(`{"X-API": "__T__", "Accept": "application/json"}`),
		Secrets: []string{"T"},
	}
	secretRepo.EXPECT().GetByName(gomock.Any(), "T").Return(&model.Secret{Name: "T", Value: "abc"}, nil)

	payload := json.RawMessage(`{"foo":1}`)
	prep, err := svc.ProcessSinkConfiguration(context.Background(), sink, payload)
	require.NoError(t, err)
	assert.Equal(t, "abc", prep.Headers["X-API"])
	assert.Equal(t, "application/json", prep.Headers["Accept"])
	assert.Equal(t, map[string]string{"__T__": "abc"}, prep.Secrets)
}

func TestAlertSinkService_ScheduleAlert(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)

	svc := NewAlertSinkService(AlertSinkServiceOptions{JobRepo: jobRepo, SecretRepo: secretRepo, Evaluator: evalStub{}})

	sink := &model.HTTPAlertSink{ID: "sink-123", Retry: 5}
	payload := json.RawMessage(`{"x":1}`)

	jobRepo.EXPECT().
		Create(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, req *model.CreateJobRequest) (*model.Job, error) {
			assert.Equal(t, model.JobTypeAlert, req.Type)
			assert.Equal(t, 5, req.MaxRetries)
			var body struct {
				SinkID  string          `json:"sink_id"`
				Payload json.RawMessage `json:"payload"`
			}
			_ = json.Unmarshal(req.Payload, &body)
			assert.Equal(t, "sink-123", body.SinkID)
			assert.JSONEq(t, `{"x":1}`, string(body.Payload))
			return &model.Job{ID: "job-1"}, nil
		})

	job, err := svc.ScheduleAlert(context.Background(), sink, payload)
	require.NoError(t, err)
	assert.Equal(t, "job-1", job.ID)
}

func TestAlertSinkService_ParseJSONHeaders_Invalid(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)
	svc := NewAlertSinkService(AlertSinkServiceOptions{JobRepo: jobRepo, SecretRepo: secretRepo, Evaluator: evalStub{}})

	sink := model.HTTPAlertSink{Method: "POST", URI: "https://example.com", Headers: ptr("{ invalid")}
	_, err := svc.ProcessSinkConfiguration(context.Background(), sink, json.RawMessage(`{}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid headers JSON")
}

func TestAlertSinkService_ParseJSONHeaders_ArrayValues(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)
	svc := NewAlertSinkService(AlertSinkServiceOptions{JobRepo: jobRepo, SecretRepo: secretRepo, Evaluator: evalStub{}})

	sink := model.HTTPAlertSink{
		Method:  "GET",
		URI:     "https://example.com",
		Headers: ptr(`{"Accept":["application/json","text/plain"]}`),
	}
	prep, err := svc.ProcessSinkConfiguration(context.Background(), sink, json.RawMessage(`{}`))
	require.NoError(t, err)
	assert.Equal(t, "application/json, text/plain", prep.Headers["Accept"])
}

func ptr(s string) *string { return &s }

// Tests for HTTPAlertSinkService (CRUD operations)

func TestNewHTTPAlertSinkService(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockHTTPAlertSinkRepository(ctrl)

	t.Run("success", func(t *testing.T) {
		svc, err := NewHTTPAlertSinkService(HTTPAlertSinkServiceOptions{
			Repo: repo,
		})
		require.NoError(t, err)
		assert.NotNil(t, svc)
	})

	t.Run("missing repo", func(t *testing.T) {
		svc, err := NewHTTPAlertSinkService(HTTPAlertSinkServiceOptions{})
		require.Error(t, err)
		assert.Nil(t, svc)
		assert.Contains(t, err.Error(), "HTTPAlertSinkRepository is required")
	})
}

func TestHTTPAlertSinkService_Create(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockHTTPAlertSinkRepository(ctrl)
	svc := MustNewHTTPAlertSinkService(HTTPAlertSinkServiceOptions{Repo: repo})

	req := &model.CreateHTTPAlertSinkRequest{
		Name:   "test-sink",
		URI:    "https://example.com",
		Method: "POST",
	}

	expected := &model.HTTPAlertSink{
		ID:     "sink-1",
		Name:   "test-sink",
		URI:    "https://example.com",
		Method: "POST",
	}

	repo.EXPECT().Create(gomock.Any(), req).Return(expected, nil)

	result, err := svc.Create(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestHTTPAlertSinkService_GetByID(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockHTTPAlertSinkRepository(ctrl)
	svc := MustNewHTTPAlertSinkService(HTTPAlertSinkServiceOptions{Repo: repo})

	expected := &model.HTTPAlertSink{
		ID:   "sink-1",
		Name: "test-sink",
	}

	repo.EXPECT().GetByID(gomock.Any(), "sink-1").Return(expected, nil)

	result, err := svc.GetByID(context.Background(), "sink-1")
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestHTTPAlertSinkService_List(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockHTTPAlertSinkRepository(ctrl)
	svc := MustNewHTTPAlertSinkService(HTTPAlertSinkServiceOptions{Repo: repo})

	expected := []*model.HTTPAlertSink{
		{ID: "sink-1", Name: "test-sink-1"},
		{ID: "sink-2", Name: "test-sink-2"},
	}

	repo.EXPECT().List(gomock.Any(), 10, 0).Return(expected, nil)

	result, err := svc.List(context.Background(), 10, 0)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestHTTPAlertSinkService_Update(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockHTTPAlertSinkRepository(ctrl)
	svc := MustNewHTTPAlertSinkService(HTTPAlertSinkServiceOptions{Repo: repo})

	req := &model.UpdateHTTPAlertSinkRequest{
		Name: ptr("updated-sink"),
	}

	expected := &model.HTTPAlertSink{
		ID:   "sink-1",
		Name: "updated-sink",
	}

	repo.EXPECT().Update(gomock.Any(), "sink-1", req).Return(expected, nil)

	result, err := svc.Update(context.Background(), "sink-1", req)
	require.NoError(t, err)
	assert.Equal(t, expected, result)
}

func TestHTTPAlertSinkService_Delete(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	repo := mocks.NewMockHTTPAlertSinkRepository(ctrl)
	svc := MustNewHTTPAlertSinkService(HTTPAlertSinkServiceOptions{Repo: repo})

	repo.EXPECT().Delete(gomock.Any(), "sink-1").Return(true, nil)

	result, err := svc.Delete(context.Background(), "sink-1")
	require.NoError(t, err)
	assert.True(t, result)
}

// mockHTTPDoer is a mock HTTP client for testing TestFire.
type mockHTTPDoer struct {
	doFunc func(req *http.Request) (*http.Response, error)
}

func (m *mockHTTPDoer) Do(req *http.Request) (*http.Response, error) {
	return m.doFunc(req)
}

func TestAlertSinkService_TestFire_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)

	svc := NewAlertSinkService(AlertSinkServiceOptions{
		JobRepo:    jobRepo,
		SecretRepo: secretRepo,
		Evaluator:  evalStub{},
	})

	sink := &model.HTTPAlertSink{
		ID:       "sink-1",
		Name:     "test-sink",
		URI:      "https://webhook.example.com/alert",
		Method:   "POST",
		OkStatus: 200,
		Secrets:  []string{},
	}

	// Mock HTTP client that returns 200 OK
	mockClient := &mockHTTPDoer{
		doFunc: func(req *http.Request) (*http.Response, error) {
			assert.Equal(t, "POST", req.Method)
			assert.Equal(t, "https://webhook.example.com/alert", req.URL.String())

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{"Content-Type": []string{"application/json"}},
				Body:       io.NopCloser(strings.NewReader(`{"status":"ok"}`)),
			}, nil
		},
	}

	result, err := svc.TestFire(context.Background(), sink, mockClient)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Success)
	assert.Equal(t, 200, result.StatusCode)
	assert.Equal(t, 200, result.ExpectedCode)
	assert.Empty(t, result.ErrorMessage)
	assert.NotNil(t, result.Response)
	assert.JSONEq(t, `{"status":"ok"}`, result.Response.Body)
}

func TestAlertSinkService_TestFire_WrongStatusCode(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)

	svc := NewAlertSinkService(AlertSinkServiceOptions{
		JobRepo:    jobRepo,
		SecretRepo: secretRepo,
		Evaluator:  evalStub{},
	})

	sink := &model.HTTPAlertSink{
		ID:       "sink-1",
		Name:     "test-sink",
		URI:      "https://webhook.example.com/alert",
		Method:   "POST",
		OkStatus: 200,
		Secrets:  []string{},
	}

	// Mock HTTP client that returns 500
	mockClient := &mockHTTPDoer{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"error":"internal server error"}`)),
			}, nil
		},
	}

	result, err := svc.TestFire(context.Background(), sink, mockClient)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.False(t, result.Success)
	assert.Equal(t, 500, result.StatusCode)
	assert.Equal(t, 200, result.ExpectedCode)
	assert.Contains(t, result.ErrorMessage, "unexpected status")
}

func TestAlertSinkService_TestFire_NetworkError(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)

	svc := NewAlertSinkService(AlertSinkServiceOptions{
		JobRepo:    jobRepo,
		SecretRepo: secretRepo,
		Evaluator:  evalStub{},
	})

	sink := &model.HTTPAlertSink{
		ID:       "sink-1",
		Name:     "test-sink",
		URI:      "https://webhook.example.com/alert",
		Method:   "POST",
		OkStatus: 200,
		Secrets:  []string{},
	}

	// Mock HTTP client that returns network error
	mockClient := &mockHTTPDoer{
		doFunc: func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		},
	}

	result, err := svc.TestFire(context.Background(), sink, mockClient)
	require.NoError(t, err) // TestFire should not return error, but capture it in result
	require.NotNil(t, result)

	assert.False(t, result.Success)
	assert.Contains(t, result.ErrorMessage, "connection refused")
}

func TestAlertSinkService_TestFire_WithSecrets(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)

	svc := NewAlertSinkService(AlertSinkServiceOptions{
		JobRepo:    jobRepo,
		SecretRepo: secretRepo,
		Evaluator:  evalStub{},
	})

	sink := &model.HTTPAlertSink{
		ID:       "sink-1",
		Name:     "test-sink",
		URI:      "https://webhook.example.com/alert?token=__API_KEY__",
		Method:   "POST",
		Headers:  ptr(`{"Authorization": "Bearer __API_KEY__"}`),
		OkStatus: 200,
		Secrets:  []string{"API_KEY"},
	}

	// Mock secret lookup
	secretRepo.EXPECT().GetByName(gomock.Any(), "API_KEY").Return(&model.Secret{
		Name:  "API_KEY",
		Value: "secret-token-123",
	}, nil)

	// Mock HTTP client that validates secret was resolved
	mockClient := &mockHTTPDoer{
		doFunc: func(req *http.Request) (*http.Response, error) {
			// Verify secret was resolved in URL
			assert.Contains(t, req.URL.String(), "token=secret-token-123")
			// Verify secret was resolved in headers
			assert.Equal(t, "Bearer secret-token-123", req.Header.Get("Authorization"))

			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader(`{"ok":true}`)),
			}, nil
		},
	}

	result, err := svc.TestFire(context.Background(), sink, mockClient)
	require.NoError(t, err)
	require.NotNil(t, result)

	assert.True(t, result.Success)
	// Verify secrets are redacted in the request summary
	assert.Contains(t, result.Request.URL, "__API_KEY__")
	assert.NotContains(t, result.Request.URL, "secret-token-123")
	// Verify Authorization header is masked (sensitive header masking)
	assert.Equal(t, "Bearer ***", result.Request.Headers["Authorization"])
	assert.NotContains(t, result.Request.Headers["Authorization"], "secret-token-123")
	assert.NotContains(t, result.Request.Headers["Authorization"], "__API_KEY__")
}

func TestAlertSinkService_TestFire_NilSink(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)

	svc := NewAlertSinkService(AlertSinkServiceOptions{
		JobRepo:    jobRepo,
		SecretRepo: secretRepo,
		Evaluator:  evalStub{},
	})

	mockClient := &mockHTTPDoer{}

	_, err := svc.TestFire(context.Background(), nil, mockClient)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "sink is required")
}

func TestAlertSinkService_TestFire_NilClient(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	secretRepo := mocks.NewMockSecretRepository(ctrl)
	jobRepo := mocks.NewMockJobRepository(ctrl)

	svc := NewAlertSinkService(AlertSinkServiceOptions{
		JobRepo:    jobRepo,
		SecretRepo: secretRepo,
		Evaluator:  evalStub{},
	})

	sink := &model.HTTPAlertSink{
		ID:   "sink-1",
		Name: "test-sink",
		URI:  "https://webhook.example.com/alert",
	}

	_, err := svc.TestFire(context.Background(), sink, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "http client is required")
}
