package rules

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

type stubCache struct {
	setIfNotExistsResp bool
	setIfNotExistsErr  error
	lastKey            string
}

func (s *stubCache) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return nil
}

func (s *stubCache) Get(ctx context.Context, key string) ([]byte, error) {
	return nil, nil
}

func (s *stubCache) Delete(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func (s *stubCache) Exists(ctx context.Context, key string) (bool, error) {
	return false, nil
}

func (s *stubCache) SetTTL(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	return false, nil
}

func (s *stubCache) SetIfNotExists(
	ctx context.Context,
	key string,
	value []byte,
	ttl time.Duration,
) (bool, error) {
	s.lastKey = key
	return s.setIfNotExistsResp, s.setIfNotExistsErr
}

func (s *stubCache) Health(ctx context.Context) error {
	return nil
}

func TestJobCoordinator_ShouldProcess(t *testing.T) {
	t.Parallel()

	ctx := context.Background()
	cache := &stubCache{setIfNotExistsResp: true}
	coord := NewJobCoordinator(JobCoordinatorOptions{
		Cache: cache,
		TTL:   time.Minute,
	})

	req := &EnqueueJobRequest{
		EventIDs: []string{"b", "a"},
		SiteID:   "site",
		Scope:    "scope",
	}

	ok, err := coord.ShouldProcess(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected should process to be true")
	}
	if cache.lastKey == "" {
		t.Fatalf("expected cache key to be recorded")
	}

	cache.setIfNotExistsResp = false
	ok, err = coord.ShouldProcess(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatalf("expected duplicate detection to return false")
	}
}

func TestJobCoordinator_BuildAndParsePayload(t *testing.T) {
	t.Parallel()

	coord := NewJobCoordinator(JobCoordinatorOptions{})
	req := &EnqueueJobRequest{
		EventIDs: []string{"1"},
		SiteID:   "site",
		Scope:    "scope",
	}
	payloadBytes, err := coord.BuildPayload(req)
	if err != nil {
		t.Fatalf("build payload: %v", err)
	}

	job := &model.Job{Payload: payloadBytes}
	payload, err := coord.ParsePayload(job)
	if err != nil {
		t.Fatalf("parse payload: %v", err)
	}
	if payload.SiteID != req.SiteID || payload.Scope != req.Scope {
		t.Fatalf("unexpected payload: %#v", payload)
	}
	if len(payload.EventIDs) != len(req.EventIDs) {
		t.Fatalf("expected %d event ids, got %d", len(req.EventIDs), len(payload.EventIDs))
	}
}

func TestJobCoordinator_LimitEventIDs(t *testing.T) {
	t.Parallel()

	coord := NewJobCoordinator(JobCoordinatorOptions{
		BatchSize: 2,
	})

	ids := coord.LimitEventIDs([]string{"1", "2", "3"}, "job")
	if len(ids) != 2 {
		t.Fatalf("expected 2 ids, got %d", len(ids))
	}

	if ids[0] != "1" || ids[1] != "2" {
		t.Fatalf("unexpected ids: %v", ids)
	}
}

func TestJobCoordinator_ShouldProcessError(t *testing.T) {
	t.Parallel()

	cache := &stubCache{setIfNotExistsErr: assertUnmarshalableError{}}
	coord := NewJobCoordinator(JobCoordinatorOptions{
		Cache: cache,
	})

	req := &EnqueueJobRequest{
		EventIDs: []string{"1"},
		SiteID:   "site",
		Scope:    "scope",
	}

	ok, err := coord.ShouldProcess(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatalf("expected should process to be true when cache errors")
	}
}

type assertUnmarshalableError struct{}

func (assertUnmarshalableError) Error() string {
	return "cache error"
}

func TestJobCoordinator_ParsePayloadError(t *testing.T) {
	t.Parallel()

	coord := NewJobCoordinator(JobCoordinatorOptions{})
	job := &model.Job{Payload: json.RawMessage(`{"event_ids": "invalid"}`)}

	_, err := coord.ParsePayload(job)
	if err == nil {
		t.Fatalf("expected parse error")
	}
}
