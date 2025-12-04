package service

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

const testDomainAllowlistID = "test-id"

// mockDomainAllowlistRepository is a mock implementation of DomainAllowlistRepository.
type mockDomainAllowlistRepository struct {
	mock.Mock
}

func (m *mockDomainAllowlistRepository) Create(
	ctx context.Context,
	req *model.CreateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.DomainAllowlist), args.Error(1)
}

func (m *mockDomainAllowlistRepository) GetByID(ctx context.Context, id string) (*model.DomainAllowlist, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.DomainAllowlist), args.Error(1)
}

func (m *mockDomainAllowlistRepository) Update(
	ctx context.Context,
	id string,
	req model.UpdateDomainAllowlistRequest,
) (*model.DomainAllowlist, error) {
	args := m.Called(ctx, id, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.DomainAllowlist), args.Error(1)
}

func (m *mockDomainAllowlistRepository) Delete(ctx context.Context, id string) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockDomainAllowlistRepository) List(
	ctx context.Context,
	opts model.DomainAllowlistListOptions,
) ([]*model.DomainAllowlist, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.DomainAllowlist), args.Error(1)
}

func (m *mockDomainAllowlistRepository) GetForScope(
	ctx context.Context,
	req model.DomainAllowlistLookupRequest,
) ([]*model.DomainAllowlist, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.DomainAllowlist), args.Error(1)
}

func (m *mockDomainAllowlistRepository) Stats(
	ctx context.Context,
	siteID *string,
) (*model.DomainAllowlistStats, error) {
	args := m.Called(ctx, siteID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.DomainAllowlistStats), args.Error(1)
}

func TestNewDomainAllowlistService(t *testing.T) {
	t.Run("success with all options", func(t *testing.T) {
		repo := &mockDomainAllowlistRepository{}
		logger := slog.Default()

		svc := NewDomainAllowlistService(DomainAllowlistServiceOptions{
			Repo:   repo,
			Logger: logger,
		})

		assert.NotNil(t, svc)
		assert.Equal(t, repo, svc.repo)
		assert.Equal(t, logger, svc.logger)
	})

	t.Run("success with minimal options", func(t *testing.T) {
		repo := &mockDomainAllowlistRepository{}

		svc := NewDomainAllowlistService(DomainAllowlistServiceOptions{
			Repo: repo,
		})

		assert.NotNil(t, svc)
		assert.Equal(t, repo, svc.repo)
		assert.Nil(t, svc.logger)
	})

	t.Run("panic when repo is nil", func(t *testing.T) {
		assert.Panics(t, func() {
			NewDomainAllowlistService(DomainAllowlistServiceOptions{})
		})
	})
}

func TestDomainAllowlistService_Create(t *testing.T) {
	ctx := context.Background()
	repo := &mockDomainAllowlistRepository{}
	svc := NewDomainAllowlistService(DomainAllowlistServiceOptions{Repo: repo})

	t.Run("success", func(t *testing.T) {
		req := &model.CreateDomainAllowlistRequest{
			Pattern: "example.com",
			Scope:   "default",
		}
		expected := &model.DomainAllowlist{
			ID:      "test-id",
			Pattern: "example.com",
			Scope:   "default",
		}

		repo.On("Create", ctx, req).Return(expected, nil).Once()

		result, err := svc.Create(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
		repo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		req := &model.CreateDomainAllowlistRequest{
			Pattern: "example.com",
			Scope:   "default",
		}
		repoErr := errors.New("database error")

		repo.On("Create", ctx, req).Return(nil, repoErr).Once()

		result, err := svc.Create(ctx, req)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "create domain allowlist")
		repo.AssertExpectations(t)
	})
}

func TestDomainAllowlistService_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := &mockDomainAllowlistRepository{}
	svc := NewDomainAllowlistService(DomainAllowlistServiceOptions{Repo: repo})

	t.Run("success", func(t *testing.T) {
		id := testDomainAllowlistID
		expected := &model.DomainAllowlist{
			ID:      id,
			Pattern: "example.com",
			Scope:   "default",
		}

		repo.On("GetByID", ctx, id).Return(expected, nil).Once()

		result, err := svc.GetByID(ctx, id)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
		repo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		id := testDomainAllowlistID
		repoErr := errors.New("not found")

		repo.On("GetByID", ctx, id).Return(nil, repoErr).Once()

		result, err := svc.GetByID(ctx, id)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "get domain allowlist by id")
		repo.AssertExpectations(t)
	})
}

func TestDomainAllowlistService_Delete(t *testing.T) {
	ctx := context.Background()
	repo := &mockDomainAllowlistRepository{}
	svc := NewDomainAllowlistService(DomainAllowlistServiceOptions{Repo: repo})

	t.Run("success", func(t *testing.T) {
		id := testDomainAllowlistID

		repo.On("Delete", ctx, id).Return(nil).Once()

		err := svc.Delete(ctx, id)

		require.NoError(t, err)
		repo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		id := testDomainAllowlistID
		repoErr := errors.New("database error")

		repo.On("Delete", ctx, id).Return(repoErr).Once()

		err := svc.Delete(ctx, id)

		require.Error(t, err)
		assert.Contains(t, err.Error(), "delete domain allowlist")
		repo.AssertExpectations(t)
	})
}
