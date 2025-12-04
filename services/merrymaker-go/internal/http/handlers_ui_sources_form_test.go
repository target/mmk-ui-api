package httpx

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/service"
	"github.com/target/mmk-ui-api/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// sourcesSvcAdapter adapts arbitrary funcs to the SourcesService interface for tests.
type sourcesSvcAdapter struct {
	createFn  func(context.Context, *model.CreateSourceRequest) (*model.Source, error)
	getByIDFn func(context.Context, string) (*model.Source, error)
	listFn    func(context.Context, int, int) ([]*model.Source, error)
	deleteFn  func(context.Context, string) (bool, error)
	resolveFn func(context.Context, *model.Source) (string, error)
}

func (a sourcesSvcAdapter) List(ctx context.Context, limit, offset int) ([]*model.Source, error) {
	return a.listFn(ctx, limit, offset)
}

func (a sourcesSvcAdapter) GetByID(ctx context.Context, id string) (*model.Source, error) {
	return a.getByIDFn(ctx, id)
}

func (a sourcesSvcAdapter) Create(ctx context.Context, req *model.CreateSourceRequest) (*model.Source, error) {
	return a.createFn(ctx, req)
}

func (a sourcesSvcAdapter) Delete(ctx context.Context, id string) (bool, error) {
	return a.deleteFn(ctx, id)
}

func (a sourcesSvcAdapter) ResolveScript(ctx context.Context, source *model.Source) (string, error) {
	if a.resolveFn != nil {
		return a.resolveFn(ctx, source)
	}
	if source == nil {
		return "", nil
	}
	return source.Value, nil
}

func (a sourcesSvcAdapter) CountJobsBySource(context.Context, string, bool) (int, error) {
	return 0, nil
}

func (a sourcesSvcAdapter) CountBrowserJobsBySource(context.Context, string, bool) (int, error) {
	return 0, nil
}

func TestUIHandlers_SourceNew(t *testing.T) {
	h := CreateUIHandlersForTest(t)
	require.NotNil(t, h)
	r := httptest.NewRequest(http.MethodGet, "/sources/new", nil)
	rr := httptest.NewRecorder()

	h.SourceNew(rr, r)

	res := rr.Result()
	t.Cleanup(func() { _ = res.Body.Close() })
	assert.Equal(t, http.StatusOK, res.StatusCode)
	assert.Contains(t, res.Header.Get("Content-Type"), "text/html")
	body := rr.Body.String()
	assert.Contains(t, body, "Source Script")
	assert.Contains(t, body, `id="source-form"`)
	assert.Contains(t, body, `data-component="source-form"`)
}

func TestUIHandlers_SourceCreate_ValidationAndSuccess(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		tr := RequireTemplateRenderer(t)
		require.NotNil(t, tr)

		sourceRepo := data.NewSourceRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		secretRepo := data.NewSecretRepo(db, cryptoutil.NoopEncryptor{})
		sourceService := service.NewSourceService(service.SourceServiceOptions{
			SourceRepo: sourceRepo,
			Jobs:       jobRepo,
			SecretRepo: secretRepo,
		})
		h := &UIHandlers{T: tr, SourceSvc: sourceService}

		t.Run("validation error - empty value", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "my-source")
			form.Set("value", "")

			r := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.SourceCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()
			assert.Contains(t, body, "Source script is required")
		})

		t.Run("success", func(t *testing.T) {
			form := url.Values{}
			form.Set("name", "my-source-1")
			form.Set("value", "console.log('hi')")

			r := httptest.NewRequest(http.MethodPost, "/sources", strings.NewReader(form.Encode()))
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			rr := httptest.NewRecorder()

			h.SourceCreate(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusNoContent, res.StatusCode)
			assert.Equal(t, "/sources", res.Header.Get("Hx-Redirect"))
		})
	})
}

func TestUIHandlers_SourceCopy(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		tr := RequireTemplateRenderer(t)
		require.NotNil(t, tr)

		sourceRepo := data.NewSourceRepo(db)
		secretRepo := data.NewSecretRepo(db, cryptoutil.NoopEncryptor{})
		// Create a source with secrets to copy
		src, err := sourceRepo.Create(context.Background(), &model.CreateSourceRequest{
			Name:    "orig-src",
			Value:   "console.log('from orig')",
			Secrets: []string{"FOO", "BAR"},
		})
		require.NoError(t, err)
		require.NotNil(t, src)

		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		sourceService := service.NewSourceService(service.SourceServiceOptions{
			SourceRepo: sourceRepo,
			Jobs:       jobRepo,
		})
		h := &UIHandlers{
			T:         tr,
			SourceSvc: sourceService,
			SecretSvc: service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo}),
		}

		r := httptest.NewRequest(http.MethodGet, "/sources/"+src.ID+"/copy", nil)
		r.SetPathValue("id", src.ID)
		rr := httptest.NewRecorder()

		h.SourceCopy(rr, r)

		res := rr.Result()
		t.Cleanup(func() { _ = res.Body.Close() })
		assert.Equal(t, http.StatusOK, res.StatusCode)
		assert.Contains(t, res.Header.Get("Content-Type"), "text/html")
		body := rr.Body.String()
		// Prefilled name/value and secrets selected
		assert.Contains(t, body, "orig-src Copy")
		assert.Contains(t, body, "console.log(&#39;from orig&#39;)")
		assert.Contains(t, body, `<option value="BAR" selected>`)
		assert.Contains(t, body, `<option value="FOO" selected>`)
		assert.Contains(t, body, `data-component="source-form"`)
	})
}

func TestUIHandlers_SourceTest_StartsJobAndRendersPanel(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		tr := RequireTemplateRenderer(t)
		require.NotNil(t, tr)

		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		sourceRepo := data.NewSourceRepo(db)
		secretRepo := data.NewSecretRepo(db, cryptoutil.NoopEncryptor{})

		jobSvc := service.MustNewJobService(service.JobServiceOptions{Repo: jobRepo, DefaultLease: 30})
		srcSvc := service.NewSourceService(service.SourceServiceOptions{SourceRepo: sourceRepo, Jobs: jobRepo})

		h := &UIHandlers{
			T: tr,
			SourceSvc: sourcesSvcAdapter{
				createFn:  srcSvc.Create,
				getByIDFn: sourceRepo.GetByID,
				listFn:    sourceRepo.List,
				deleteFn:  sourceRepo.Delete,
				resolveFn: srcSvc.ResolveScript,
			},
			Jobs:      jobSvc,
			SecretSvc: service.MustNewSecretService(service.SecretServiceOptions{Repo: secretRepo}),
		}

		form := url.Values{}
		form.Set("name", "tmp-src")
		form.Set("value", "console.log('hi')")
		form.Add("secrets", "FOO")

		r := httptest.NewRequest(http.MethodPost, "/sources/test", strings.NewReader(form.Encode()))
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		rr := httptest.NewRecorder()

		h.SourceTest(rr, r)

		res := rr.Result()
		t.Cleanup(func() { _ = res.Body.Close() })
		assert.Equal(t, http.StatusOK, res.StatusCode)
		body := rr.Body.String()
		// Panel is present and includes source/job context
		assert.Contains(t, body, `id="source-test-panel"`)
		assert.Contains(t, body, `id="test-status"`)
		assert.Contains(t, body, `data-source-id=`)
		// Either waiting or has a Job ID with status badge
		assert.True(
			t,
			strings.Contains(body, "Waiting") || strings.Contains(body, "Loading") ||
				strings.Contains(body, "status-badge"),
		)
		assert.Contains(t, body, `data-component="source-form"`)
	})
}

func TestUIHandlers_SourceTestEvents_NoDuplicates(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		tr := RequireTemplateRenderer(t)
		require.NotNil(t, tr)

		ctx := context.Background()
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		eventRepo := &data.EventRepo{DB: db}

		// Create a test job
		job, err := jobRepo.Create(ctx, &model.CreateJobRequest{
			Type:     model.JobTypeBrowser,
			Payload:  []byte(`{"url":"https://example.com"}`),
			Priority: 10,
		})
		require.NoError(t, err)

		// Create 10 test events for the job using BulkInsert
		sessionID := "550e8400-e29b-41d4-a716-446655440000"
		jobID := job.ID
		events := make([]model.RawEvent, 10)
		for i := range 10 {
			events[i] = model.RawEvent{
				Type: "console.log",
				Data: []byte(`{"message":"test event"}`),
			}
		}
		_, err = eventRepo.BulkInsert(ctx, model.BulkEventRequest{
			SessionID:   sessionID,
			SourceJobID: &jobID,
			Events:      events,
		}, false)
		require.NoError(t, err)

		eventSvc := service.MustNewEventService(service.EventServiceOptions{
			Repo: eventRepo,
			Config: service.EventServiceConfig{
				MaxBatch:                 100,
				ThreatScoreProcessCutoff: 0.7,
			},
		})
		jobSvc := service.MustNewJobService(service.JobServiceOptions{Repo: jobRepo, DefaultLease: 30})

		h := &UIHandlers{T: tr, EventSvc: eventSvc, Jobs: jobSvc}

		t.Run("first request with since=0 returns all 10 events", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/sources/test/"+job.ID+"/events?since=0", nil)
			r.SetPathValue("id", job.ID)
			rr := httptest.NewRecorder()

			h.SourceTestEvents(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()

			// Should contain polling state update with next-since=10
			assert.Contains(t, body, `data-next-since="10"`)
			assert.Contains(t, body, `class="polling-state-update"`)

			// Count event-card occurrences (should be exactly 10)
			eventCardCount := strings.Count(body, `class="event-card`)
			assert.Equal(t, 10, eventCardCount, "Expected exactly 10 event cards")
		})

		t.Run("second request with since=10 returns no new events", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/sources/test/"+job.ID+"/events?since=10", nil)
			r.SetPathValue("id", job.ID)
			rr := httptest.NewRecorder()

			h.SourceTestEvents(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()

			// Should still have next-since=10 (no new events)
			assert.Contains(t, body, `data-next-since="10"`)

			// Should have NO event cards
			eventCardCount := strings.Count(body, `class="event-card`)
			assert.Equal(t, 0, eventCardCount, "Expected no event cards when no new events")
		})

		t.Run("duplicate request with since=0 returns same events again", func(t *testing.T) {
			// This simulates the bug scenario where multiple polling intervals
			// both start from since=0
			r := httptest.NewRequest(http.MethodGet, "/sources/test/"+job.ID+"/events?since=0", nil)
			r.SetPathValue("id", job.ID)
			rr := httptest.NewRecorder()

			h.SourceTestEvents(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()

			// Server correctly returns the same 10 events again
			// (This is expected server behavior - the client must track offset correctly)
			assert.Contains(t, body, `data-next-since="10"`)
			eventCardCount := strings.Count(body, `class="event-card`)
			assert.Equal(t, 10, eventCardCount, "Server should return same events for same offset")
		})

		t.Run("pagination with since=5 returns events 5-9", func(t *testing.T) {
			r := httptest.NewRequest(http.MethodGet, "/sources/test/"+job.ID+"/events?since=5", nil)
			r.SetPathValue("id", job.ID)
			rr := httptest.NewRecorder()

			h.SourceTestEvents(rr, r)

			res := rr.Result()
			t.Cleanup(func() { _ = res.Body.Close() })
			assert.Equal(t, http.StatusOK, res.StatusCode)
			body := rr.Body.String()

			// Should return 5 events (indices 5-9)
			assert.Contains(t, body, `data-next-since="10"`)
			eventCardCount := strings.Count(body, `class="event-card`)
			assert.Equal(t, 5, eventCardCount, "Expected 5 events from offset 5")
		})
	})
}
