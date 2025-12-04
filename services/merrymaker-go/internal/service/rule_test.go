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

const testRuleID = "test-rule-id"

// mockRuleRepository is a mock implementation of RuleRepository.
type mockRuleRepository struct {
	mock.Mock
}

func (m *mockRuleRepository) Create(
	ctx context.Context,
	req model.CreateRuleRequest,
) (*model.Rule, error) {
	args := m.Called(ctx, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Rule), args.Error(1)
}

func (m *mockRuleRepository) GetByID(ctx context.Context, id string) (*model.Rule, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Rule), args.Error(1)
}

func (m *mockRuleRepository) List(
	ctx context.Context,
	opts model.RuleListOptions,
) ([]*model.Rule, error) {
	args := m.Called(ctx, opts)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Rule), args.Error(1)
}

func (m *mockRuleRepository) Update(
	ctx context.Context,
	id string,
	req model.UpdateRuleRequest,
) (*model.Rule, error) {
	args := m.Called(ctx, id, req)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Rule), args.Error(1)
}

func (m *mockRuleRepository) Delete(ctx context.Context, id string) (bool, error) {
	args := m.Called(ctx, id)
	return args.Bool(0), args.Error(1)
}

func (m *mockRuleRepository) GetBySite(
	ctx context.Context,
	siteID string,
	enabled *bool,
) ([]*model.Rule, error) {
	args := m.Called(ctx, siteID, enabled)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model.Rule), args.Error(1)
}

func TestNewRuleService(t *testing.T) {
	t.Run("success with all options", func(t *testing.T) {
		repo := &mockRuleRepository{}
		logger := slog.Default()

		svc, err := NewRuleService(RuleServiceOptions{
			Repo:   repo,
			Logger: logger,
		})

		require.NoError(t, err)
		assert.NotNil(t, svc)
		assert.Equal(t, repo, svc.repo)
		assert.Equal(t, logger, svc.logger)
	})

	t.Run("success with minimal options", func(t *testing.T) {
		repo := &mockRuleRepository{}

		svc, err := NewRuleService(RuleServiceOptions{
			Repo: repo,
		})

		require.NoError(t, err)
		assert.NotNil(t, svc)
		assert.Equal(t, repo, svc.repo)
		assert.Nil(t, svc.logger)
	})

	t.Run("error when repo is nil", func(t *testing.T) {
		svc, err := NewRuleService(RuleServiceOptions{})

		assert.Nil(t, svc)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "RuleRepository is required")
	})
}

func TestRuleService_Create(t *testing.T) {
	ctx := context.Background()
	repo := &mockRuleRepository{}
	svc, err := NewRuleService(RuleServiceOptions{Repo: repo})
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		req := model.CreateRuleRequest{
			SiteID:   "test-site-id",
			RuleType: "unknown_domain",
		}
		expected := &model.Rule{
			ID:       testRuleID,
			SiteID:   "test-site-id",
			RuleType: "unknown_domain",
		}

		repo.On("Create", ctx, req).Return(expected, nil).Once()

		result, err := svc.Create(ctx, req)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
		repo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		req := model.CreateRuleRequest{
			SiteID:   "test-site-id",
			RuleType: "unknown_domain",
		}
		repoErr := errors.New("database error")

		repo.On("Create", ctx, req).Return(nil, repoErr).Once()

		result, err := svc.Create(ctx, req)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "create rule")
		repo.AssertExpectations(t)
	})
}

func TestRuleService_GetByID(t *testing.T) {
	ctx := context.Background()
	repo := &mockRuleRepository{}
	svc, err := NewRuleService(RuleServiceOptions{Repo: repo})
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		id := testRuleID
		expected := &model.Rule{
			ID:       id,
			SiteID:   "test-site-id",
			RuleType: "unknown_domain",
		}

		repo.On("GetByID", ctx, id).Return(expected, nil).Once()

		result, err := svc.GetByID(ctx, id)

		require.NoError(t, err)
		assert.Equal(t, expected, result)
		repo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		id := testRuleID
		repoErr := errors.New("not found")

		repo.On("GetByID", ctx, id).Return(nil, repoErr).Once()

		result, err := svc.GetByID(ctx, id)

		require.Error(t, err)
		assert.Nil(t, result)
		assert.Contains(t, err.Error(), "get rule by id")
		repo.AssertExpectations(t)
	})
}

func TestRuleService_Delete(t *testing.T) {
	ctx := context.Background()
	repo := &mockRuleRepository{}
	svc, err := NewRuleService(RuleServiceOptions{Repo: repo})
	require.NoError(t, err)

	t.Run("success", func(t *testing.T) {
		id := testRuleID

		repo.On("Delete", ctx, id).Return(true, nil).Once()

		result, err := svc.Delete(ctx, id)

		require.NoError(t, err)
		assert.True(t, result)
		repo.AssertExpectations(t)
	})

	t.Run("repository error", func(t *testing.T) {
		id := testRuleID
		repoErr := errors.New("database error")

		repo.On("Delete", ctx, id).Return(false, repoErr).Once()

		result, err := svc.Delete(ctx, id)

		require.Error(t, err)
		assert.False(t, result)
		assert.Contains(t, err.Error(), "delete rule")
		repo.AssertExpectations(t)
	})
}
