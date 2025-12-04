package rules

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type captureMetrics struct{ events []CacheEvent }

func (c *captureMetrics) RecordCacheEvent(e CacheEvent) { c.events = append(c.events, e) }

type fakeCacheRepo struct{ m map[string][]byte }

func (f *fakeCacheRepo) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	if f.m == nil {
		f.m = map[string][]byte{}
	}
	f.m[key] = value
	return nil
}
func (f *fakeCacheRepo) Get(_ context.Context, key string) ([]byte, error) { return f.m[key], nil }
func (f *fakeCacheRepo) Delete(_ context.Context, key string) (bool, error) {
	delete(f.m, key)
	return true, nil
}

func (f *fakeCacheRepo) Exists(_ context.Context, key string) (bool, error) {
	_, ok := f.m[key]
	return ok, nil
}

func (f *fakeCacheRepo) SetTTL(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (f *fakeCacheRepo) SetIfNotExists(_ context.Context, key string, value []byte, _ time.Duration) (bool, error) {
	if _, ok := f.m[key]; ok {
		return false, nil
	}
	if f.m == nil {
		f.m = map[string][]byte{}
	}
	f.m[key] = value
	return true, nil
}
func (f *fakeCacheRepo) Health(_ context.Context) error { return nil }

type fakeProcessedRepo struct{ m map[string]bool }

func (f *fakeProcessedRepo) Create(
	ctx context.Context,
	req model.CreateProcessedFileRequest,
) (*model.ProcessedFile, error) {
	if f.m == nil {
		f.m = map[string]bool{}
	}
	k := req.SiteID + "|" + strings.ToLower(req.Scope) + "|" + strings.ToLower(req.FileHash)
	f.m[k] = true
	return &model.ProcessedFile{
		ID:         "1",
		SiteID:     req.SiteID,
		FileHash:   req.FileHash,
		StorageKey: req.StorageKey,
		Scope:      req.Scope,
	}, nil
}

func (f *fakeProcessedRepo) GetByID(ctx context.Context, id string) (*model.ProcessedFile, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeProcessedRepo) List(
	ctx context.Context,
	opts model.ProcessedFileListOptions,
) ([]*model.ProcessedFile, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeProcessedRepo) Update(
	ctx context.Context,
	id string,
	req model.UpdateProcessedFileRequest,
) (*model.ProcessedFile, error) {
	return nil, errors.New("not implemented")
}

func (f *fakeProcessedRepo) Delete(ctx context.Context, id string) (bool, error) {
	return false, errors.New("not implemented")
}

func (f *fakeProcessedRepo) Lookup(
	ctx context.Context,
	req model.ProcessedFileLookupRequest,
) (*model.ProcessedFile, error) {
	k := req.SiteID + "|" + strings.ToLower(req.Scope) + "|" + strings.ToLower(req.FileHash)
	if f.m != nil && f.m[k] {
		return &model.ProcessedFile{ID: "1", SiteID: req.SiteID, FileHash: req.FileHash, Scope: req.Scope}, nil
	}
	return nil, nil //nolint:nilnil // returning (nil, nil) to indicate not found is expected here
}

func (f *fakeProcessedRepo) Stats(ctx context.Context, siteID *string) (*model.ProcessedFileStats, error) {
	return &model.ProcessedFileStats{}, nil
}

func TestProcessedFilesCache_MarkProcessed_MetricsWrites(t *testing.T) {
	ctx := context.Background()
	metrics := &captureMetrics{}
	redis := &fakeCacheRepo{m: map[string][]byte{}}
	repo := &fakeProcessedRepo{m: map[string]bool{}}
	local := NewLocalLRU(LocalLRUConfig{Capacity: 10, Now: time.Now})
	c := NewProcessedFilesCache(ProcessedFilesCacheDeps{
		Local:   local,
		Redis:   redis,
		Repo:    repo,
		TTL:     DefaultCacheTTL(),
		Metrics: metrics,
	})

	key := FileKey{Scope: ScopeKey{SiteID: "site-1", Scope: "default"}, FileHash: strings.Repeat("a", 64)}
	require.NoError(t, c.MarkProcessed(ctx, key, "s3://bucket/key"))

	var sawRedisWrite, sawLocalWrite bool
	for _, e := range metrics.events {
		if e.Name == CacheFiles && e.Op == OpWrite && e.Tier == TierRedis {
			sawRedisWrite = true
		}
		if e.Name == CacheFiles && e.Op == OpWrite && e.Tier == TierLocal {
			sawLocalWrite = true
		}
	}
	assert.True(t, sawRedisWrite, "expected redis write event")
	assert.True(t, sawLocalWrite, "expected local write event")
}
