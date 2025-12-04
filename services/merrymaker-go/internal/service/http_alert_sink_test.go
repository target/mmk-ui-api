package service

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
