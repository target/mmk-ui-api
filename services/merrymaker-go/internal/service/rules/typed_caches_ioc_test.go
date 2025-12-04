package rules

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/stretchr/testify/require"
)

type stubIOCRepo struct {
	mu     sync.Mutex
	lookup func(context.Context, model.IOCLookupRequest) (*model.IOC, error)
}

func newStubIOCRepo() *stubIOCRepo {
	return &stubIOCRepo{
		lookup: func(context.Context, model.IOCLookupRequest) (*model.IOC, error) {
			return nil, ErrNotFound
		},
	}
}

func (s *stubIOCRepo) withLookup(fn func(context.Context, model.IOCLookupRequest) (*model.IOC, error)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lookup = fn
}

func (s *stubIOCRepo) LookupHost(ctx context.Context, req model.IOCLookupRequest) (*model.IOC, error) {
	s.mu.Lock()
	fn := s.lookup
	s.mu.Unlock()
	return fn(ctx, req)
}

// Unused interface methods.
func (s *stubIOCRepo) Create(context.Context, model.CreateIOCRequest) (*model.IOC, error) {
	panic("not implemented")
}

func (s *stubIOCRepo) GetByID(context.Context, string) (*model.IOC, error) {
	panic("not implemented")
}

func (s *stubIOCRepo) List(context.Context, model.IOCListOptions) ([]*model.IOC, error) {
	panic("not implemented")
}

func (s *stubIOCRepo) Update(context.Context, string, model.UpdateIOCRequest) (*model.IOC, error) {
	panic("not implemented")
}

func (s *stubIOCRepo) Delete(context.Context, string) (bool, error) {
	panic("not implemented")
}

func (s *stubIOCRepo) BulkCreate(context.Context, model.BulkCreateIOCsRequest) (int, error) {
	panic("not implemented")
}

func (s *stubIOCRepo) Stats(context.Context) (*core.IOCStats, error) {
	panic("not implemented")
}

var _ core.IOCRepository = (*stubIOCRepo)(nil)

func TestIOCCacheLookupHostInvalidatedAfterVersionBump(t *testing.T) {
	ctx := context.Background()
	repo := newStubIOCRepo()

	versioner := NewIOCCacheVersioner(nil, "", time.Millisecond)
	cache := NewIOCCache(IOCCacheDeps{
		Local:     NewLocalLRU(LocalLRUConfig{Capacity: 32, Now: time.Now}),
		Redis:     nil,
		Repo:      repo,
		TTL:       DefaultCacheTTL(),
		Metrics:   NoopCacheMetrics{},
		Versioner: versioner,
	})

	host := "www.targetecards.com"

	repo.withLookup(func(context.Context, model.IOCLookupRequest) (*model.IOC, error) {
		return nil, ErrNotFound
	})

	_, err := cache.LookupHost(ctx, host)
	require.ErrorIs(t, err, ErrNotFound)

	// Introduce a matching IOC and bump the version to clear negative cache entries.
	repo.withLookup(func(context.Context, model.IOCLookupRequest) (*model.IOC, error) {
		return &model.IOC{
			ID:      "ioc-1",
			Type:    model.IOCTypeFQDN,
			Value:   "*.targetecards.com",
			Enabled: true,
		}, nil
	})

	_, bumpErr := versioner.Bump(ctx)
	require.NoError(t, bumpErr)

	ioc, err := cache.LookupHost(ctx, host)
	require.NoError(t, err)
	require.NotNil(t, ioc)
	require.Equal(t, "ioc-1", ioc.ID)
}
