package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/testutil"
)

// fakeCacheRepo is a simple in-memory CacheRepository for integration testing.
type fakeCacheRepo struct{ m map[string][]byte }

func (f *fakeCacheRepo) Set(_ context.Context, key string, value []byte, _ time.Duration) error {
	if f.m == nil {
		f.m = make(map[string][]byte)
	}
	f.m[key] = append([]byte(nil), value...)
	return nil
}
func (f *fakeCacheRepo) Get(_ context.Context, key string) ([]byte, error) { return f.m[key], nil }
func (f *fakeCacheRepo) Delete(_ context.Context, key string) (bool, error) {
	if _, ok := f.m[key]; ok {
		delete(f.m, key)
		return true, nil
	}
	return false, nil
}

func (f *fakeCacheRepo) Exists(_ context.Context, key string) (bool, error) {
	_, ok := f.m[key]
	return ok, nil
}

func (f *fakeCacheRepo) SetTTL(_ context.Context, _ string, _ time.Duration) (bool, error) {
	return true, nil
}

func (f *fakeCacheRepo) SetIfNotExists(_ context.Context, key string, value []byte, _ time.Duration) (bool, error) {
	if _, exists := f.m[key]; exists {
		return false, nil // Key already exists
	}
	f.m[key] = value
	return true, nil // Key was set
}
func (f *fakeCacheRepo) Health(_ context.Context) error { return nil }

func TestSourceService_Create_TestSource_AutoEnqueue_Integration(t *testing.T) {
	testutil.SkipIfNoTestDB(t)
	testutil.WithAutoDB(t, func(db *sql.DB) {
		ctx := context.Background()

		// Repos
		sourceRepo := data.NewSourceRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		secretRepo := data.NewSecretRepo(db, cryptoutil.NoopEncryptor{})

		// Fake cache + cache service
		fc := &fakeCacheRepo{m: make(map[string][]byte)}
		cacheSvc := core.NewSourceCacheService(core.SourceCacheServiceOptions{
			Cache:   fc,
			Sources: sourceRepo,
			Secrets: secretRepo,
			Config:  core.DefaultSourceCacheConfig(),
		})

		// Service under test
		svc := NewSourceService(
			SourceServiceOptions{SourceRepo: sourceRepo, Jobs: jobRepo, Cache: cacheSvc, SecretRepo: secretRepo},
		)

		// Create a test source
		req := &model.CreateSourceRequest{Name: "src-intg", Value: "console.log('ok')", Test: true}
		src, err := svc.Create(ctx, req)
		require.NoError(t, err)
		require.NotNil(t, src)

		// Verify job was enqueued and linked to source with IsTest=true
		jobs, err := jobRepo.ListBySource(ctx, model.JobListBySourceOptions{SourceID: src.ID, Limit: 10, Offset: 0})
		require.NoError(t, err)
		require.Len(t, jobs, 1)
		j := jobs[0]
		assert.Equal(t, model.JobTypeBrowser, j.Type)
		assert.True(t, j.IsTest)
		if assert.NotNil(t, j.SourceID) {
			assert.Equal(t, src.ID, *j.SourceID)
		}
		// Payload should contain source_id
		var p struct {
			SourceID string `json:"source_id"`
		}
		require.NoError(t, json.Unmarshal(j.Payload, &p))
		assert.Equal(t, src.ID, p.SourceID)

		// Verify source content cached via service/cache
		cached, err := cacheSvc.GetCachedSourceContent(ctx, src.ID)
		require.NoError(t, err)
		require.NotNil(t, cached)
		assert.Equal(t, []byte(src.Value), cached)
	})
}
