package httpx

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/testhelpers"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
)

func createJobHTTPTime(t testutil.TestingTB, baseURL string, req *model.CreateJobRequest) model.Job {
	t.Helper()
	resp := DoJSON(t, JSONRequest{
		Method:  http.MethodPost,
		URL:     baseURL + "/api/jobs",
		Payload: req,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("create job status: %d", resp.StatusCode)
	}
	var out model.Job
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode create job: %v", err)
	}
	return out
}

func reserveNextHTTPTime(t testutil.TestingTB, baseURL string, leaseSec int) model.Job {
	t.Helper()
	url := fmt.Sprintf("%s/api/jobs/%s/reserve_next?lease=%d&wait=%d", baseURL, model.JobTypeBrowser, leaseSec, 0)
	resp := DoJSON(t, JSONRequest{
		Method: http.MethodGet,
		URL:    url,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("reserve_next status: %d", resp.StatusCode)
	}
	var out model.Job
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		t.Fatalf("decode reserved job: %v", err)
	}
	return out
}

type heartbeatConfig struct {
	BaseURL   string
	JobID     string
	ExtendSec int
}

func heartbeatHTTPTime(t testutil.TestingTB, cfg heartbeatConfig) {
	t.Helper()
	url := fmt.Sprintf("%s/api/jobs/%s/heartbeat?extend=%d", cfg.BaseURL, cfg.JobID, cfg.ExtendSec)
	resp := DoJSON(t, JSONRequest{
		Method: http.MethodPost,
		URL:    url,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("heartbeat status: %d", resp.StatusCode)
	}
}

type failJobConfig struct {
	BaseURL string
	JobID   string
	ErrMsg  string
}

func failJobHTTPTime(t testutil.TestingTB, cfg failJobConfig) {
	t.Helper()
	url := fmt.Sprintf("%s/api/jobs/%s/fail", cfg.BaseURL, cfg.JobID)
	payload := map[string]string{"error": cfg.ErrMsg}
	resp := DoJSON(t, JSONRequest{
		Method:  http.MethodPost,
		URL:     url,
		Payload: payload,
	})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("fail status: %d", resp.StatusCode)
	}
}

// newServerWithFixedTime wires production services/handlers with a FixedTimeProvider-backed JobRepo.
func newServerWithFixedTime(
	t *testing.T,
	db *sql.DB,
	tp *data.FixedTimeProvider,
	cfg data.RepoConfig,
) (*httptest.Server, *data.JobRepo) {
	t.Helper()
	jobRepo := testhelpers.NewJobRepoWithTimeProvider(db, cfg, tp)
	eventRepo := &data.EventRepo{DB: db}
	jobSvc := service.MustNewJobService(service.JobServiceOptions{
		Repo:         jobRepo,
		DefaultLease: 30 * time.Second,
	})
	eventSvc := service.MustNewEventService(service.EventServiceOptions{
		Repo: eventRepo,
		Config: service.EventServiceConfig{
			MaxBatch:                 1000,
			ThreatScoreProcessCutoff: 0.7,
		},
	})
	mux := NewRouter(RouterServices{Jobs: jobSvc, Events: eventSvc, IsDev: true})
	return httptest.NewServer(mux), jobRepo
}

func Test_Workflow_LeaseExpiry_Requeue_viaREST_WithFixedTime(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Fixed time starting point
		start := testutil.TestTime()
		tp := data.NewFixedTimeProvider(start)
		ts, _ := newServerWithFixedTime(t, db, tp, data.RepoConfig{RetryDelaySeconds: 5})
		defer ts.Close()

		// Create and reserve a job with a short lease
		created := createJobHTTPTime(t, ts.URL, &model.CreateJobRequest{
			Type:       model.JobTypeBrowser,
			Payload:    json.RawMessage(`{"a":1}`),
			MaxRetries: 1,
		})
		reserved := reserveNextHTTPTime(t, ts.URL, 1)
		if reserved.ID != created.ID {
			t.Fatalf("reserved mismatch: got %s want %s", reserved.ID, created.ID)
		}

		// Advance beyond lease expiry and reserve again -> requeued opportunistically
		tp.AddTime(2 * time.Second)
		again := reserveNextHTTPTime(t, ts.URL, 30)
		if again.ID != created.ID {
			t.Fatalf("requeue failed: got %s want %s", again.ID, created.ID)
		}
	})
}

func Test_Workflow_PeriodicHeartbeats_ExtendLease_viaREST_WithFixedTime(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		start := testutil.TestTime()
		tp := data.NewFixedTimeProvider(start)
		ts, jobRepo := newServerWithFixedTime(t, db, tp, data.RepoConfig{})
		defer ts.Close()

		created := createJobHTTPTime(
			t,
			ts.URL,
			&model.CreateJobRequest{Type: model.JobTypeBrowser, Payload: json.RawMessage(`{"b":2}`)},
		)
		reserved := reserveNextHTTPTime(t, ts.URL, 5)
		if reserved.ID != created.ID {
			t.Fatalf("reserved mismatch: got %s want %s", reserved.ID, created.ID)
		}

		// After 3s, heartbeat extend by 5s => new lease = (start+3s)+5s
		tp.AddTime(3 * time.Second)
		heartbeatHTTPTime(t, heartbeatConfig{
			BaseURL:   ts.URL,
			JobID:     reserved.ID,
			ExtendSec: 5,
		})
		job, err := jobRepo.GetByID(context.Background(), reserved.ID)
		if err != nil {
			t.Fatalf("get job: %v", err)
		}
		if job.LeaseExpiresAt == nil {
			t.Fatalf("lease_expires_at nil after heartbeat")
		}
		expected := tp.Now().Add(5 * time.Second)
		if !job.LeaseExpiresAt.Equal(expected.UTC()) {
			t.Fatalf("lease not extended: got %v want %v", job.LeaseExpiresAt, expected.UTC())
		}

		// Advance near expiration and heartbeat again
		tp.AddTime(4 * time.Second) // 1s before expiry
		heartbeatHTTPTime(t, heartbeatConfig{
			BaseURL:   ts.URL,
			JobID:     reserved.ID,
			ExtendSec: 10,
		})
		job2, err := jobRepo.GetByID(context.Background(), reserved.ID)
		if err != nil {
			t.Fatalf("get job2: %v", err)
		}
		expected2 := tp.Now().Add(10 * time.Second)
		if job2.LeaseExpiresAt == nil || !job2.LeaseExpiresAt.Equal(expected2.UTC()) {
			t.Fatalf("second heartbeat not applied: got %v want %v", job2.LeaseExpiresAt, expected2.UTC())
		}
	})
}

func Test_Workflow_FailAndRetryScheduling_viaREST_WithFixedTime(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		start := testutil.TestTime()
		tp := data.NewFixedTimeProvider(start)
		retryDelay := 5
		ts, jobRepo := newServerWithFixedTime(t, db, tp, data.RepoConfig{RetryDelaySeconds: retryDelay})
		defer ts.Close()

		// Create, reserve, then fail once -> pending with scheduled_at in future
		created := createJobHTTPTime(
			t,
			ts.URL,
			&model.CreateJobRequest{Type: model.JobTypeBrowser, Payload: json.RawMessage(`{"c":3}`), MaxRetries: 2},
		)
		reserved := reserveNextHTTPTime(t, ts.URL, 30)
		if reserved.ID != created.ID {
			t.Fatalf("reserved mismatch: got %s want %s", reserved.ID, created.ID)
		}
		failJobHTTPTime(t, failJobConfig{
			BaseURL: ts.URL,
			JobID:   reserved.ID,
			ErrMsg:  "first failure",
		})

		j1, err := jobRepo.GetByID(context.Background(), reserved.ID)
		if err != nil {
			t.Fatalf("get job after fail: %v", err)
		}
		if j1.Status != model.JobStatusPending {
			t.Fatalf("after first fail, status=%s want pending", j1.Status)
		}
		expectedSchedule := tp.Now().Add(time.Duration(retryDelay) * time.Second)
		if !j1.ScheduledAt.Equal(expectedSchedule.UTC()) {
			t.Fatalf("scheduled_at not set correctly: got %v want %v", j1.ScheduledAt, expectedSchedule.UTC())
		}

		// Advance past retry delay and reserve again
		tp.AddTime(time.Duration(retryDelay+1) * time.Second)
		again := reserveNextHTTPTime(t, ts.URL, 30)
		if again.ID != created.ID {
			t.Fatalf("did not retry: got %s want %s", again.ID, created.ID)
		}

		// Fail again -> exceeds max retries -> terminal failed
		failJobHTTPTime(t, failJobConfig{
			BaseURL: ts.URL,
			JobID:   again.ID,
			ErrMsg:  "second failure",
		})
		j2, err := jobRepo.GetByID(context.Background(), again.ID)
		if err != nil {
			t.Fatalf("get job after second fail: %v", err)
		}
		if j2.Status != model.JobStatusFailed {
			t.Fatalf("after second fail, status=%s want failed", j2.Status)
		}
		if j2.CompletedAt == nil {
			t.Fatalf("expected completed_at set on terminal failure")
		}
	})
}
