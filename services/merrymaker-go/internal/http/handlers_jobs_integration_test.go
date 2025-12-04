package httpx

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
)

// jobRepoAdapter adapts data.JobRepo to core.JobRepository for integration wiring.
type jobRepoAdapter struct{ r *data.JobRepo }

func (a *jobRepoAdapter) Create(
	ctx context.Context,
	req *model.CreateJobRequest,
) (*model.Job, error) {
	return a.r.Create(ctx, req)
}

func (a *jobRepoAdapter) GetByID(ctx context.Context, id string) (*model.Job, error) {
	return a.r.GetByID(ctx, id)
}

func (a *jobRepoAdapter) ReserveNext(
	ctx context.Context,
	jobType model.JobType,
	leaseSeconds int,
) (*model.Job, error) {
	return a.r.ReserveNext(ctx, jobType, leaseSeconds)
}

func (a *jobRepoAdapter) WaitForNotification(ctx context.Context, jobType model.JobType) error {
	return a.r.WaitForNotification(ctx, jobType)
}

func (a *jobRepoAdapter) Heartbeat(
	ctx context.Context,
	jobID string,
	leaseSeconds int,
) (bool, error) {
	return a.r.Heartbeat(ctx, jobID, leaseSeconds)
}

func (a *jobRepoAdapter) Complete(ctx context.Context, id string) (bool, error) {
	return a.r.Complete(ctx, id)
}

func (a *jobRepoAdapter) Fail(ctx context.Context, id, errMsg string) (bool, error) {
	return a.r.Fail(ctx, id, errMsg)
}

func (a *jobRepoAdapter) Stats(
	ctx context.Context,
	jobType model.JobType,
) (*model.JobStats, error) {
	return a.r.Stats(ctx, jobType)
}

func (a *jobRepoAdapter) List(ctx context.Context, opts *model.JobListOptions) ([]*model.JobWithEventCount, error) {
	return a.r.List(ctx, opts)
}

func (a *jobRepoAdapter) Delete(ctx context.Context, id string) error {
	return a.r.Delete(ctx, id)
}

func (a *jobRepoAdapter) DeleteByPayloadField(
	ctx context.Context,
	params core.DeleteByPayloadFieldParams,
) (int, error) {
	return a.r.DeleteByPayloadField(ctx, params)
}

// TestReserveNext_LongPoll_ReceivesJob is an integration test that exercises
// the long-poll path end-to-end against Postgres notifications.
func TestReserveNext_LongPoll_ReceivesJob(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := data.NewJobRepo(db, data.RepoConfig{})
		svc := service.MustNewJobService(service.JobServiceOptions{
			Repo:         &jobRepoAdapter{r: repo},
			DefaultLease: 30 * time.Second,
		})
		h := &JobHandlers{Svc: svc}

		// Start a long-poll reserve request that should unblock after we create a job.
		w := httptest.NewRecorder()
		r := httptest.NewRequest(
			http.MethodGet,
			"/api/jobs/rules/reserve_next?lease=15&wait=5",
			nil,
		)
		r.SetPathValue("type", "rules")

		done := make(chan struct{})
		go func() {
			h.ReserveNext(w, r)
			close(done)
		}()

		// Give the background listener time to start LISTEN; small delay is fine.
		time.Sleep(250 * time.Millisecond)

		// Create a job which should notify listeners and wake the long-poll.
		created, err := repo.Create(context.Background(), &model.CreateJobRequest{
			Type:    model.JobTypeRules,
			Payload: json.RawMessage(`{"x":1}`),
		})
		require.NoError(t, err)

		// Wait for handler to complete or timeout.
		select {
		case <-done:
		case <-time.After(6 * time.Second):
			t.Fatal("timeout waiting for ReserveNext to return")
		}

		resp := w.Result()
		defer resp.Body.Close()
		require.Equal(t, http.StatusOK, resp.StatusCode)

		var got model.Job
		err = json.NewDecoder(resp.Body).Decode(&got)
		require.NoError(t, err)
		require.Equal(t, created.ID, got.ID)

		// Ensure no stray listeners remain.
		svc.StopAllListeners()
	})
}

func TestJobHandlers_GetStatus_Integration(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		repo := data.NewJobRepo(db, data.RepoConfig{})
		svc := service.MustNewJobService(service.JobServiceOptions{
			Repo:         &jobRepoAdapter{r: repo},
			DefaultLease: 30 * time.Second,
		})
		h := &JobHandlers{Svc: svc}

		// Create a job
		createReq := &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Priority: 10,
			Payload:  json.RawMessage(`{"test": "data"}`),
		}
		job, err := repo.Create(context.Background(), createReq)
		require.NoError(t, err)

		// Test getting status of pending job
		w := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "/api/jobs/"+job.ID+"/status", nil)
		r.SetPathValue("id", job.ID)

		h.GetStatus(w, r)

		require.Equal(t, http.StatusOK, w.Code)

		var response model.JobStatusResponse
		err = json.Unmarshal(w.Body.Bytes(), &response)
		require.NoError(t, err)
		require.Equal(t, model.JobStatusPending, response.Status)
		require.Nil(t, response.CompletedAt)
		require.Nil(t, response.LastError)

		// Reserve the job first (jobs can only be completed if they're running)
		reserved, err := repo.ReserveNext(context.Background(), model.JobTypeBrowser, 30)
		require.NoError(t, err)
		require.NotNil(t, reserved)
		require.Equal(t, job.ID, reserved.ID)

		// Complete the job
		success, err := repo.Complete(context.Background(), job.ID)
		require.NoError(t, err)
		require.True(t, success)

		// Test getting status of completed job
		w2 := httptest.NewRecorder()
		r2 := httptest.NewRequest(http.MethodGet, "/api/jobs/"+job.ID+"/status", nil)
		r2.SetPathValue("id", job.ID)

		h.GetStatus(w2, r2)

		require.Equal(t, http.StatusOK, w2.Code)

		var response2 model.JobStatusResponse
		err = json.Unmarshal(w2.Body.Bytes(), &response2)
		require.NoError(t, err)
		require.Equal(t, model.JobStatusCompleted, response2.Status)
		require.NotNil(t, response2.CompletedAt)
		require.Nil(t, response2.LastError)

		// Test getting status of nonexistent job (use valid UUID format)
		w3 := httptest.NewRecorder()
		r3 := httptest.NewRequest(http.MethodGet, "/api/jobs/00000000-0000-0000-0000-000000000000/status", nil)
		r3.SetPathValue("id", "00000000-0000-0000-0000-000000000000")

		h.GetStatus(w3, r3)

		require.Equal(t, http.StatusNotFound, w3.Code)
	})
}
