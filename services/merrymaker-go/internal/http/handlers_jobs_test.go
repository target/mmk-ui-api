package httpx

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"github.com/target/mmk-ui-api/internal/service"
	"go.uber.org/mock/gomock"
)

func newHandlersWithMock(
	t *testing.T,
) (*JobHandlers, *mocks.MockJobRepository, *gomock.Controller) {
	t.Helper()
	ctrl := gomock.NewController(t)
	mockRepo := mocks.NewMockJobRepository(ctrl)
	svc := service.MustNewJobService(service.JobServiceOptions{
		Repo:         mockRepo,
		DefaultLease: 30 * time.Second,
	})
	return &JobHandlers{Svc: svc}, mockRepo, ctrl
}

func TestCreateJob_Success(t *testing.T) {
	h, mockRepo, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	reqBody := model.CreateJobRequest{
		Type:    model.JobTypeBrowser,
		Payload: json.RawMessage(`{"url":"https://example.com"}`),
	}
	expected := &model.Job{
		ID:      "job-123",
		Type:    model.JobTypeBrowser,
		Status:  model.JobStatusPending,
		Payload: reqBody.Payload,
	}

	mockRepo.EXPECT().Create(gomock.Any(), gomock.Any()).Return(expected, nil)

	b, _ := json.Marshal(reqBody)
	r := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewReader(b))
	w := httptest.NewRecorder()

	h.CreateJob(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got model.Job
	err := json.NewDecoder(resp.Body).Decode(&got)
	require.NoError(t, err)
	assert.Equal(t, expected.ID, got.ID)
}

func TestCreateJob_InvalidJSON(t *testing.T) {
	h, _, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	r := httptest.NewRequest(http.MethodPost, "/api/jobs", bytes.NewBufferString("{bad"))
	w := httptest.NewRecorder()

	h.CreateJob(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestReserveNext_Success(t *testing.T) {
	h, mockRepo, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	expected := &model.Job{
		ID:     "job-abc",
		Type:   model.JobTypeBrowser,
		Status: model.JobStatusRunning,
	}

	mockRepo.EXPECT().ReserveNext(gomock.Any(), model.JobTypeBrowser, 45).Return(expected, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/jobs/browser/reserve_next?lease=45", nil)
	r.SetPathValue("type", "browser")
	w := httptest.NewRecorder()

	h.ReserveNext(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got model.Job
	err := json.NewDecoder(resp.Body).Decode(&got)
	require.NoError(t, err)
	assert.Equal(t, expected.ID, got.ID)
}

func TestReserveNext_NoJob_NoWait_Returns204(t *testing.T) {
	h, mockRepo, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	mockRepo.EXPECT().
		ReserveNext(gomock.Any(), model.JobTypeBrowser, 30).
		Return(nil, model.ErrNoJobsAvailable)

	r := httptest.NewRequest(http.MethodGet, "/api/jobs/browser/reserve_next", nil)
	r.SetPathValue("type", "browser")
	w := httptest.NewRecorder()

	h.ReserveNext(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusNoContent, resp.StatusCode)
}

func TestReserveNext_Error_Returns400(t *testing.T) {
	h, mockRepo, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	mockRepo.EXPECT().
		ReserveNext(gomock.Any(), model.JobTypeBrowser, 30).
		Return(nil, assert.AnError)

	r := httptest.NewRequest(http.MethodGet, "/api/jobs/browser/reserve_next", nil)
	r.SetPathValue("type", "browser")
	w := httptest.NewRecorder()

	h.ReserveNext(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestHeartbeat_Success(t *testing.T) {
	h, mockRepo, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	mockRepo.EXPECT().Heartbeat(gomock.Any(), "job-1", 10).Return(true, nil)

	r := httptest.NewRequest(http.MethodPost, "/api/jobs/job-1/heartbeat?extend=10", nil)
	r.SetPathValue("id", "job-1")
	w := httptest.NewRecorder()

	h.Heartbeat(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got map[string]bool
	err := json.NewDecoder(resp.Body).Decode(&got)
	require.NoError(t, err)
	assert.True(t, got["ok"])
}

func TestComplete_Success(t *testing.T) {
	h, mockRepo, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	mockRepo.EXPECT().Complete(gomock.Any(), "job-2").Return(true, nil)
	r := httptest.NewRequest(http.MethodPost, "/api/jobs/job-2/complete", nil)
	r.SetPathValue("id", "job-2")
	w := httptest.NewRecorder()

	h.Complete(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestFail_Success(t *testing.T) {
	h, mockRepo, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	mockRepo.EXPECT().Fail(gomock.Any(), "job-3", "bad").Return(true, nil)

	body := map[string]string{"error": "bad"}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/jobs/job-3/fail", bytes.NewReader(b))
	r.SetPathValue("id", "job-3")
	w := httptest.NewRecorder()

	h.Fail(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
}

func TestFail_EmptyMessage_Returns400(t *testing.T) {
	h, _, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	body := map[string]string{"error": ""}
	b, _ := json.Marshal(body)
	r := httptest.NewRequest(http.MethodPost, "/api/jobs/job-4/fail", bytes.NewReader(b))
	r.SetPathValue("id", "job-4")
	w := httptest.NewRecorder()

	h.Fail(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusBadRequest, resp.StatusCode)
}

func TestStats_Success(t *testing.T) {
	h, mockRepo, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	expected := &model.JobStats{Pending: 1, Running: 2, Completed: 3, Failed: 0}
	mockRepo.EXPECT().Stats(gomock.Any(), model.JobTypeBrowser).Return(expected, nil)

	r := httptest.NewRequest(http.MethodGet, "/api/jobs/browser/stats", nil)
	r.SetPathValue("type", "browser")
	w := httptest.NewRecorder()

	h.Stats(w, r)

	resp := w.Result()
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)

	var got model.JobStats
	err := json.NewDecoder(resp.Body).Decode(&got)
	require.NoError(t, err)
	assert.Equal(t, expected.Completed, got.Completed)
}

func TestJobHandlers_GetStatus(t *testing.T) {
	h, mockRepo, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	jobID := "test-job-id"
	completedAt := time.Now().Truncate(time.Microsecond) // Remove monotonic clock for comparison
	lastError := "test error"

	job := &model.Job{
		ID:          jobID,
		Status:      model.JobStatusCompleted,
		CompletedAt: &completedAt,
		LastError:   &lastError,
	}

	mockRepo.EXPECT().GetByID(gomock.Any(), jobID).Return(job, nil)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID+"/status", nil)
	r.SetPathValue("id", jobID)

	h.GetStatus(w, r)

	assert.Equal(t, http.StatusOK, w.Code)

	var response model.JobStatusResponse
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, model.JobStatusCompleted, response.Status)
	assert.True(t, completedAt.Equal(*response.CompletedAt))
	assert.Equal(t, lastError, *response.LastError)
}

func TestJobHandlers_GetStatus_NotFound(t *testing.T) {
	h, mockRepo, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	jobID := "nonexistent-job-id"

	mockRepo.EXPECT().GetByID(gomock.Any(), jobID).Return(nil, data.ErrJobNotFound)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID+"/status", nil)
	r.SetPathValue("id", jobID)

	h.GetStatus(w, r)

	assert.Equal(t, http.StatusNotFound, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "job_not_found", response["error"])
	assert.Equal(t, "job not found", response["message"])
}

func TestJobHandlers_GetStatus_DatabaseError(t *testing.T) {
	h, mockRepo, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	jobID := "test-job-id"
	// Simulate a database connection error
	dbErr := errors.New("database connection failed")

	mockRepo.EXPECT().GetByID(gomock.Any(), jobID).Return(nil, dbErr)

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/jobs/"+jobID+"/status", nil)
	r.SetPathValue("id", jobID)

	h.GetStatus(w, r)

	assert.Equal(t, http.StatusInternalServerError, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "get_status_failed", response["error"])
	assert.Equal(t, "failed to get job status", response["message"])
}

func TestJobHandlers_GetStatus_MissingID(t *testing.T) {
	h, _, ctrl := newHandlersWithMock(t)
	defer ctrl.Finish()

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/jobs//status", nil)
	// Don't set path value to simulate missing ID

	h.GetStatus(w, r)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	require.NoError(t, err)
	assert.Equal(t, "invalid_path", response["error"])
}
