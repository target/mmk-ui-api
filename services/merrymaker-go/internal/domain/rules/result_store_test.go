package rules

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

type recordingCache struct {
	store map[string][]byte
	ttl   time.Duration
	err   error
}

func (c *recordingCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	if c.store == nil {
		c.store = make(map[string][]byte)
	}
	c.store[key] = append([]byte(nil), value...)
	c.ttl = ttl
	return c.err
}

func (c *recordingCache) Get(ctx context.Context, key string) ([]byte, error) {
	return c.store[key], c.err
}

func (c *recordingCache) Delete(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func (c *recordingCache) Exists(ctx context.Context, key string) (bool, error) {
	_, ok := c.store[key]
	return ok, nil
}

func (c *recordingCache) SetTTL(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return false, nil
}

func (c *recordingCache) SetIfNotExists(
	ctx context.Context,
	key string,
	value []byte,
	ttl time.Duration,
) (bool, error) {
	return false, nil
}

func (c *recordingCache) Health(ctx context.Context) error {
	return nil
}

type recordingJobResultRepo struct {
	upserts []core.UpsertJobResultParams
	record  *model.JobResult
	getErr  error
}

func (r *recordingJobResultRepo) Upsert(
	ctx context.Context,
	params core.UpsertJobResultParams,
) error {
	r.upserts = append(r.upserts, params)
	return nil
}

func (r *recordingJobResultRepo) GetByJobID(
	ctx context.Context,
	jobID string,
) (*model.JobResult, error) {
	return r.record, r.getErr
}

func (r *recordingJobResultRepo) ListByAlertID(
	ctx context.Context,
	alertID string,
) ([]*model.JobResult, error) {
	return nil, nil
}

func TestResultStoreCache(t *testing.T) {
	t.Parallel()

	cache := &recordingCache{}
	store := NewResultStore(ResultStoreOptions{
		Cache:    cache,
		CacheTTL: time.Hour,
	})

	err := store.Cache(context.Background(), "job-1", &ProcessingResults{})
	if err != nil {
		t.Fatalf("cache: %v", err)
	}
	payload, ok := cache.store["rules:results:job-1"]
	if !ok {
		t.Fatalf("expected cache entry")
	}
	if cache.ttl != time.Hour {
		t.Fatalf("expected ttl hour, got %v", cache.ttl)
	}
	if len(payload) == 0 {
		t.Fatalf("expected payload stored")
	}
}

func TestResultStorePersist(t *testing.T) {
	t.Parallel()

	repo := &recordingJobResultRepo{}
	store := NewResultStore(ResultStoreOptions{
		Repository: repo,
		JobType:    model.JobTypeRules,
	})

	job := &model.Job{ID: "job-1"}
	err := store.Persist(context.Background(), job, &ProcessingResults{})
	if err != nil {
		t.Fatalf("persist: %v", err)
	}
	if len(repo.upserts) != 1 {
		t.Fatalf("expected upsert call")
	}
	if repo.upserts[0].JobType != model.JobTypeRules {
		t.Fatalf("expected job type rules, got %s", repo.upserts[0].JobType)
	}
}

func TestResultStoreGetFromCache(t *testing.T) {
	t.Parallel()

	res := &ProcessingResults{AlertsCreated: 2}
	payload, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	cache := &recordingCache{store: map[string][]byte{
		"rules:results:job-1": payload,
	}}

	store := NewResultStore(ResultStoreOptions{
		Cache: cache,
	})

	out, err := store.Get(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.AlertsCreated != res.AlertsCreated {
		t.Fatalf("expected alerts created %d, got %d", res.AlertsCreated, out.AlertsCreated)
	}
}

func TestResultStoreGetFromRepository(t *testing.T) {
	t.Parallel()

	res := &ProcessingResults{AlertsCreated: 1}
	payload, err := json.Marshal(res)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	jobID := "job-1"
	repo := &recordingJobResultRepo{
		record: &model.JobResult{
			JobID:   &jobID,
			JobType: model.JobTypeRules,
			Result:  payload,
		},
	}

	store := NewResultStore(ResultStoreOptions{
		Repository: repo,
	})

	out, err := store.Get(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.AlertsCreated != res.AlertsCreated {
		t.Fatalf("expected alerts created %d, got %d", res.AlertsCreated, out.AlertsCreated)
	}
}

func TestResultStoreGetNotFound(t *testing.T) {
	t.Parallel()

	notFoundErr := errors.New("not found")
	repo := &recordingJobResultRepo{
		getErr: notFoundErr,
	}

	store := NewResultStore(ResultStoreOptions{
		Repository: repo,
		IsNotFound: func(err error) bool {
			return errors.Is(err, notFoundErr)
		},
	})

	_, err := store.Get(context.Background(), "job-1")
	if !errors.Is(err, ErrResultsNotFound) {
		t.Fatalf("expected ErrResultsNotFound, got %v", err)
	}
}
