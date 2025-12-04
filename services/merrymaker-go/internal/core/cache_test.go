package core

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"go.uber.org/mock/gomock"
)

//go:generate mockgen -source=cache.go -destination=cache_mock.go -package=core
//go:generate mockgen -destination=source_repository_mock_test.go -package=core github.com/target/mmk-ui-api/internal/core SourceRepository

func TestSourceCacheService_CacheSourceContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sourceID string
		setup    func(*MockCacheRepository, *MockSourceRepository)
		wantErr  bool
	}{
		{
			name:     "empty source ID",
			sourceID: "",
			setup:    func(*MockCacheRepository, *MockSourceRepository) {},
			wantErr:  false,
		},
		{
			name:     "cached value up-to-date skips set",
			sourceID: "source-123",
			setup: func(cache *MockCacheRepository, sources *MockSourceRepository) {
				cache.EXPECT().
					Get(gomock.Any(), "source:content:source-123").
					Return([]byte("console.log('test');"), nil)
				sources.EXPECT().GetByID(gomock.Any(), "source-123").Return(&model.Source{
					ID:    "source-123",
					Value: "console.log('test');",
				}, nil)
			},
			wantErr: false,
		},
		{
			name:     "cache miss - fetch and cache",
			sourceID: "source-123",
			setup: func(cache *MockCacheRepository, sources *MockSourceRepository) {
				cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, nil)
				sources.EXPECT().GetByID(gomock.Any(), "source-123").Return(&model.Source{
					ID:    "source-123",
					Value: "console.log('test');",
				}, nil)
				cache.EXPECT().
					Set(gomock.Any(), "source:content:source-123", []byte("console.log('test');"), 30*time.Minute).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name:     "stale cached value refreshed",
			sourceID: "source-123",
			setup: func(cache *MockCacheRepository, sources *MockSourceRepository) {
				cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return([]byte("console.log('old');"), nil)
				sources.EXPECT().GetByID(gomock.Any(), "source-123").Return(&model.Source{
					ID:    "source-123",
					Value: "console.log('new');",
				}, nil)
				cache.EXPECT().
					Set(gomock.Any(), "source:content:source-123", []byte("console.log('new');"), 30*time.Minute).
					Return(nil)
			},
			wantErr: false,
		},
		{
			name:     "cache get error",
			sourceID: "source-123",
			setup: func(cache *MockCacheRepository, sources *MockSourceRepository) {
				sources.EXPECT().GetByID(gomock.Any(), "source-123").Return(&model.Source{
					ID:    "source-123",
					Value: "console.log('test');",
				}, nil)
				cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, errors.New("redis error"))
			},
			wantErr: true,
		},
		{
			name:     "source fetch error",
			sourceID: "source-123",
			setup: func(_ *MockCacheRepository, sources *MockSourceRepository) {
				sources.EXPECT().GetByID(gomock.Any(), "source-123").Return(nil, errors.New("source not found"))
			},
			wantErr: true,
		},
		{
			name:     "cache set error",
			sourceID: "source-123",
			setup: func(cache *MockCacheRepository, sources *MockSourceRepository) {
				cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, nil)
				sources.EXPECT().GetByID(gomock.Any(), "source-123").Return(&model.Source{
					ID:    "source-123",
					Value: "console.log('test');",
				}, nil)
				cache.EXPECT().
					Set(gomock.Any(), "source:content:source-123", []byte("console.log('test');"), 30*time.Minute).
					Return(errors.New("redis error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			cache := NewMockCacheRepository(ctrl)
			sources := NewMockSourceRepository(ctrl)
			tt.setup(cache, sources)

			service := NewSourceCacheService(SourceCacheServiceOptions{
				Cache:   cache,
				Sources: sources,
				Secrets: nil,
				Config:  DefaultSourceCacheConfig(),
			})
			err := service.CacheSourceContent(context.Background(), tt.sourceID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSourceCacheService_CacheSourceContent_ResolvesSecrets(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cache := NewMockCacheRepository(ctrl)
	sources := NewMockSourceRepository(ctrl)
	secrets := newStubSecretRepo(map[string]*model.Secret{
		"API_KEY": {Name: "API_KEY", Value: "resolved-value"},
	}, nil)

	cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, nil)
	sources.EXPECT().GetByID(gomock.Any(), "source-123").Return(&model.Source{
		ID:      "source-123",
		Value:   "console.log('__API_KEY__');",
		Secrets: []string{"API_KEY"},
	}, nil)
	cache.EXPECT().Set(
		gomock.Any(),
		"source:content:source-123",
		[]byte("console.log('resolved-value');"),
		30*time.Minute,
	).Return(nil)

	service := NewSourceCacheService(SourceCacheServiceOptions{
		Cache:   cache,
		Sources: sources,
		Secrets: secrets,
		Config:  DefaultSourceCacheConfig(),
	})
	err := service.CacheSourceContent(context.Background(), "source-123")
	require.NoError(t, err)
}

func TestSourceCacheService_CacheSourceContent_SecretResolutionError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cache := NewMockCacheRepository(ctrl)
	sources := NewMockSourceRepository(ctrl)
	secrets := newStubSecretRepo(nil, errors.New("lookup failed"))

	cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, nil)
	sources.EXPECT().GetByID(gomock.Any(), "source-123").Return(&model.Source{
		ID:      "source-123",
		Value:   "console.log('__API_KEY__');",
		Secrets: []string{"API_KEY"},
	}, nil)

	service := NewSourceCacheService(SourceCacheServiceOptions{
		Cache:   cache,
		Sources: sources,
		Secrets: secrets,
		Config:  DefaultSourceCacheConfig(),
	})
	err := service.CacheSourceContent(context.Background(), "source-123")
	require.Error(t, err)
}

func TestSourceCacheService_CacheSourceContent_ReplacesPlaceholdersWhenCached(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cache := NewMockCacheRepository(ctrl)
	sources := NewMockSourceRepository(ctrl)
	secrets := newStubSecretRepo(map[string]*model.Secret{
		"API_KEY": {Name: "API_KEY", Value: "resolved"},
	}, nil)

	cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return([]byte("console.log('__API_KEY__');"), nil)
	sources.EXPECT().GetByID(gomock.Any(), "source-123").Return(&model.Source{
		ID:      "source-123",
		Value:   "console.log('__API_KEY__');",
		Secrets: []string{"API_KEY"},
	}, nil)
	cache.EXPECT().Set(
		gomock.Any(),
		"source:content:source-123",
		[]byte("console.log('resolved');"),
		30*time.Minute,
	).Return(nil)

	service := NewSourceCacheService(SourceCacheServiceOptions{
		Cache:   cache,
		Sources: sources,
		Secrets: secrets,
		Config:  DefaultSourceCacheConfig(),
	})
	err := service.CacheSourceContent(context.Background(), "source-123")
	require.NoError(t, err)
}

func TestSourceCacheService_CacheSourceContent_MissingSecretRepo(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cache := NewMockCacheRepository(ctrl)
	sources := NewMockSourceRepository(ctrl)

	cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, nil)
	sources.EXPECT().GetByID(gomock.Any(), "source-123").Return(&model.Source{
		ID:      "source-123",
		Value:   "console.log('__API_KEY__');",
		Secrets: []string{"API_KEY"},
	}, nil)

	service := NewSourceCacheService(SourceCacheServiceOptions{
		Cache:   cache,
		Sources: sources,
		Secrets: nil,
		Config:  DefaultSourceCacheConfig(),
	})
	err := service.CacheSourceContent(context.Background(), "source-123")
	require.Error(t, err)
}

func TestSourceCacheService_CacheResolvedSourceContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		source  *model.Source
		secrets SecretRepository
		setup   func(cache *MockCacheRepository)
		wantErr bool
	}{
		{
			name:    "nil source no-op",
			source:  nil,
			secrets: nil,
			setup:   func(*MockCacheRepository) {},
			wantErr: false,
		},
		{
			name:    "empty source id no-op",
			source:  &model.Source{ID: "", Value: "console.log('x')"},
			secrets: nil,
			setup:   func(*MockCacheRepository) {},
			wantErr: false,
		},
		{
			name:   "cached value up-to-date skips set",
			source: &model.Source{ID: "source-123", Value: "console.log('test');"},
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().
					Get(gomock.Any(), "source:content:source-123").
					Return([]byte("console.log('test');"), nil)
			},
		},
		{
			name:   "cache miss caches value",
			source: &model.Source{ID: "source-123", Value: "console.log('test');"},
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, nil)
				cache.EXPECT().Set(
					gomock.Any(),
					"source:content:source-123",
					[]byte("console.log('test');"),
					30*time.Minute,
				).Return(nil)
			},
		},
		{
			name:   "cache get error surfaces",
			source: &model.Source{ID: "source-123", Value: "console.log('test');"},
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().
					Get(gomock.Any(), "source:content:source-123").
					Return(nil, errors.New("redis get failed"))
			},
			wantErr: true,
		},
		{
			name:   "cache set error surfaces",
			source: &model.Source{ID: "source-123", Value: "console.log('test');"},
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, nil)
				cache.EXPECT().Set(
					gomock.Any(),
					"source:content:source-123",
					[]byte("console.log('test');"),
					30*time.Minute,
				).Return(errors.New("redis set failed"))
			},
			wantErr: true,
		},
		{
			name:   "missing secret repository returns error when secrets required",
			source: &model.Source{ID: "source-123", Value: "console.log('__API_KEY__');", Secrets: []string{"API_KEY"}},
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, nil)
			},
			wantErr: true,
		},
		{
			name:   "resolves secrets and caches resolved value",
			source: &model.Source{ID: "source-123", Value: "console.log('__API_KEY__');", Secrets: []string{"API_KEY"}},
			secrets: newStubSecretRepo(map[string]*model.Secret{
				"API_KEY": {Name: "API_KEY", Value: "resolved"},
			}, nil),
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, nil)
				cache.EXPECT().Set(
					gomock.Any(),
					"source:content:source-123",
					[]byte("console.log('resolved');"),
					30*time.Minute,
				).Return(nil)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			cache := NewMockCacheRepository(ctrl)
			sources := NewMockSourceRepository(ctrl)

			sources.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)
			if tt.setup != nil {
				tt.setup(cache)
			}

			service := NewSourceCacheService(SourceCacheServiceOptions{
				Cache:   cache,
				Sources: sources,
				Secrets: tt.secrets,
				Config:  DefaultSourceCacheConfig(),
			})

			err := service.CacheResolvedSourceContent(context.Background(), tt.source)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestSourceCacheService_GetCachedSourceContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sourceID string
		setup    func(*MockCacheRepository)
		want     []byte
		wantErr  bool
	}{
		{
			name:     "empty source ID",
			sourceID: "",
			setup:    func(*MockCacheRepository) {},
			want:     nil,
			wantErr:  false,
		},
		{
			name:     "cache hit",
			sourceID: "source-123",
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().
					Get(gomock.Any(), "source:content:source-123").
					Return([]byte("console.log('cached');"), nil)
			},
			want:    []byte("console.log('cached');"),
			wantErr: false,
		},
		{
			name:     "cache miss",
			sourceID: "source-123",
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, nil)
			},
			want:    nil,
			wantErr: false,
		},
		{
			name:     "cache error",
			sourceID: "source-123",
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().Get(gomock.Any(), "source:content:source-123").Return(nil, errors.New("redis error"))
			},
			want:    nil,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			cache := NewMockCacheRepository(ctrl)
			sources := NewMockSourceRepository(ctrl)
			tt.setup(cache)

			service := NewSourceCacheService(SourceCacheServiceOptions{
				Cache:   cache,
				Sources: sources,
				Secrets: nil,
				Config:  DefaultSourceCacheConfig(),
			})
			result, err := service.GetCachedSourceContent(context.Background(), tt.sourceID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.want, result)
			}
		})
	}
}

func TestSourceCacheService_InvalidateSourceContent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		sourceID string
		setup    func(*MockCacheRepository)
		wantErr  bool
	}{
		{
			name:     "empty source ID",
			sourceID: "",
			setup:    func(*MockCacheRepository) {},
			wantErr:  false,
		},
		{
			name:     "successful deletion",
			sourceID: "source-123",
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().Delete(gomock.Any(), "source:content:source-123").Return(true, nil)
			},
			wantErr: false,
		},
		{
			name:     "key not found",
			sourceID: "source-123",
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().Delete(gomock.Any(), "source:content:source-123").Return(false, nil)
			},
			wantErr: false,
		},
		{
			name:     "cache error",
			sourceID: "source-123",
			setup: func(cache *MockCacheRepository) {
				cache.EXPECT().
					Delete(gomock.Any(), "source:content:source-123").
					Return(false, errors.New("redis error"))
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			cache := NewMockCacheRepository(ctrl)
			sources := NewMockSourceRepository(ctrl)
			tt.setup(cache)

			service := NewSourceCacheService(SourceCacheServiceOptions{
				Cache:   cache,
				Sources: sources,
				Secrets: nil,
				Config:  DefaultSourceCacheConfig(),
			})
			err := service.InvalidateSourceContent(context.Background(), tt.sourceID)

			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDefaultSourceCacheConfig(t *testing.T) {
	t.Parallel()

	cfg := DefaultSourceCacheConfig()
	assert.Equal(t, 30*time.Minute, cfg.TTL)
}

func TestSourceCacheService_sourceContentKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	cache := NewMockCacheRepository(ctrl)
	sources := NewMockSourceRepository(ctrl)
	service := NewSourceCacheService(SourceCacheServiceOptions{
		Cache:   cache,
		Sources: sources,
		Secrets: nil,
		Config:  DefaultSourceCacheConfig(),
	})

	key := service.sourceContentKey("test-id")
	assert.Equal(t, "source:content:test-id", key)
}
