// This file is a documentation template and should not be compiled.
// It uses placeholder types (ExampleService, ExampleRepository, etc.) that don't exist.
// Use this as a reference when writing tests for services.
//
//go:build ignore

package service

// TEMPLATE_test.go - Service Testing Pattern Examples
//
// This file demonstrates standard testing patterns for services.
// Use these patterns when writing tests for new or migrated services.

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"go.uber.org/mock/gomock"
)

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 1: Constructor Tests
// ═══════════════════════════════════════════════════════════════════════════

func TestNewExampleService_RequiredDependency(t *testing.T) {
	// Test that constructor panics when required dependency is nil
	assert.Panics(t, func() {
		NewExampleService(ExampleServiceOptions{
			Repo: nil, // Required dependency is nil
		})
	})
}

func TestNewExampleService_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockExampleRepository(ctrl)

	// Test that constructor succeeds with valid dependencies
	svc := NewExampleService(ExampleServiceOptions{
		Repo: mockRepo,
	})

	assert.NotNil(t, svc)
}

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 2: Simple CRUD Tests (with Mocks)
// ═══════════════════════════════════════════════════════════════════════════

func TestExampleService_Create_Success(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockExampleRepository(ctrl)
	svc := NewExampleService(ExampleServiceOptions{Repo: mockRepo})

	ctx := context.Background()
	req := model.CreateExampleRequest{
		Name:  "test-example",
		Value: "test-value",
	}
	expected := &model.Example{
		ID:    "example-1",
		Name:  "test-example",
		Value: "test-value",
	}

	// Expectations
	mockRepo.EXPECT().
		Create(ctx, req).
		Return(expected, nil).
		Times(1)

	// Execute
	got, err := svc.Create(ctx, req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestExampleService_Create_RepositoryError(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockExampleRepository(ctrl)
	svc := NewExampleService(ExampleServiceOptions{Repo: mockRepo})

	ctx := context.Background()
	req := model.CreateExampleRequest{Name: "test"}
	repoErr := errors.New("database connection failed")

	// Expectations
	mockRepo.EXPECT().
		Create(ctx, req).
		Return(nil, repoErr).
		Times(1)

	// Execute
	got, err := svc.Create(ctx, req)

	// Assert
	require.Error(t, err)
	assert.Nil(t, got)
	assert.Contains(t, err.Error(), "create example")
	assert.ErrorIs(t, err, repoErr)
}

func TestExampleService_GetByID_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockExampleRepository(ctrl)
	svc := NewExampleService(ExampleServiceOptions{Repo: mockRepo})

	ctx := context.Background()
	id := "example-1"
	expected := &model.Example{ID: id, Name: "test"}

	mockRepo.EXPECT().
		GetByID(ctx, id).
		Return(expected, nil)

	got, err := svc.GetByID(ctx, id)

	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestExampleService_List_NormalizesPagination(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockExampleRepository(ctrl)
	svc := NewExampleService(ExampleServiceOptions{Repo: mockRepo})

	ctx := context.Background()
	expected := []*model.Example{
		{ID: "1", Name: "example1"},
		{ID: "2", Name: "example2"},
	}

	// Test that service normalizes invalid limit to default (50)
	mockRepo.EXPECT().
		List(ctx, 50, 0). // Service should normalize 0 to 50
		Return(expected, nil)

	got, err := svc.List(ctx, 0, 0) // Pass invalid limit

	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestExampleService_Update_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockExampleRepository(ctrl)
	svc := NewExampleService(ExampleServiceOptions{Repo: mockRepo})

	ctx := context.Background()
	id := "example-1"
	req := model.UpdateExampleRequest{Name: "updated"}
	expected := &model.Example{ID: id, Name: "updated"}

	mockRepo.EXPECT().
		Update(ctx, id, req).
		Return(expected, nil)

	got, err := svc.Update(ctx, id, req)

	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestExampleService_Delete_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockExampleRepository(ctrl)
	svc := NewExampleService(ExampleServiceOptions{Repo: mockRepo})

	ctx := context.Background()
	id := "example-1"

	mockRepo.EXPECT().
		Delete(ctx, id).
		Return(true, nil)

	deleted, err := svc.Delete(ctx, id)

	require.NoError(t, err)
	assert.True(t, deleted)
}

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 3: Orchestration Tests (Multiple Mocks)
// ═══════════════════════════════════════════════════════════════════════════

func TestExampleService_CreateWithRelated_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockExampleRepository(ctrl)
	svc := NewExampleService(ExampleServiceOptions{Repo: mockRepo})

	ctx := context.Background()
	req := model.CreateExampleWithRelatedRequest{
		Name:              "test",
		Value:             "value",
		AutoCreateRelated: true,
	}
	created := &model.Example{ID: "example-1", Name: "test"}

	// Expect main entity creation
	mockRepo.EXPECT().
		Create(ctx, gomock.AssignableToTypeOf(model.CreateExampleRequest{})).
		Return(created, nil)

	// Note: In a real test, you'd also mock the related entity creation
	// This demonstrates testing orchestration across multiple operations

	got, err := svc.CreateWithRelated(ctx, req)

	require.NoError(t, err)
	assert.Equal(t, created, got)
}

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 4: Optional Dependency Tests
// ═══════════════════════════════════════════════════════════════════════════

func TestExampleService_GetByID_WithCache_CacheHit(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockExampleRepository(ctrl)
	mockCache := mocks.NewMockExampleCache(ctrl)
	svc := NewExampleService(ExampleServiceOptions{
		Repo:  mockRepo,
		Cache: mockCache,
	})

	ctx := context.Background()
	id := "example-1"
	cached := &model.Example{ID: id, Name: "cached"}

	// Expect cache check (cache hit)
	mockCache.EXPECT().
		Get(ctx, gomock.Any()).
		Return(cached, nil)

	// Repository should NOT be called (cache hit)
	mockRepo.EXPECT().GetByID(gomock.Any(), gomock.Any()).Times(0)

	got, err := svc.GetByID(ctx, id)

	require.NoError(t, err)
	assert.Equal(t, cached, got)
}

func TestExampleService_GetByID_WithCache_CacheMiss(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockExampleRepository(ctrl)
	mockCache := mocks.NewMockExampleCache(ctrl)
	svc := NewExampleService(ExampleServiceOptions{
		Repo:  mockRepo,
		Cache: mockCache,
	})

	ctx := context.Background()
	id := "example-1"
	fromDB := &model.Example{ID: id, Name: "from-db"}

	// Expect cache check (cache miss)
	mockCache.EXPECT().
		Get(ctx, gomock.Any()).
		Return(nil, errors.New("cache miss"))

	// Repository should be called
	mockRepo.EXPECT().
		GetByID(ctx, id).
		Return(fromDB, nil)

	// Expect cache set (best-effort, ignore errors)
	mockCache.EXPECT().
		Set(ctx, gomock.Any(), fromDB).
		Return(nil)

	got, err := svc.GetByID(ctx, id)

	require.NoError(t, err)
	assert.Equal(t, fromDB, got)
}

func TestExampleService_GetByID_WithoutCache(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockExampleRepository(ctrl)
	svc := NewExampleService(ExampleServiceOptions{
		Repo:  mockRepo,
		Cache: nil, // No cache
	})

	ctx := context.Background()
	id := "example-1"
	fromDB := &model.Example{ID: id, Name: "from-db"}

	// Repository should be called directly (no cache)
	mockRepo.EXPECT().
		GetByID(ctx, id).
		Return(fromDB, nil)

	got, err := svc.GetByID(ctx, id)

	require.NoError(t, err)
	assert.Equal(t, fromDB, got)
}

// ═══════════════════════════════════════════════════════════════════════════
// PATTERN 5: Table-Driven Tests
// ═══════════════════════════════════════════════════════════════════════════

func TestExampleService_List_PaginationNormalization(t *testing.T) {
	tests := []struct {
		name           string
		inputLimit     int
		inputOffset    int
		expectedLimit  int
		expectedOffset int
	}{
		{
			name:           "zero limit defaults to 50",
			inputLimit:     0,
			inputOffset:    0,
			expectedLimit:  50,
			expectedOffset: 0,
		},
		{
			name:           "negative limit defaults to 50",
			inputLimit:     -10,
			inputOffset:    0,
			expectedLimit:  50,
			expectedOffset: 0,
		},
		{
			name:           "limit over 1000 capped to 1000",
			inputLimit:     5000,
			inputOffset:    0,
			expectedLimit:  1000,
			expectedOffset: 0,
		},
		{
			name:           "valid limit passed through",
			inputLimit:     100,
			inputOffset:    50,
			expectedLimit:  100,
			expectedOffset: 50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockRepo := mocks.NewMockExampleRepository(ctrl)
			svc := NewExampleService(ExampleServiceOptions{Repo: mockRepo})

			ctx := context.Background()
			expected := []*model.Example{}

			// Verify service normalizes to expected values
			mockRepo.EXPECT().
				List(ctx, tt.expectedLimit, tt.expectedOffset).
				Return(expected, nil)

			got, err := svc.List(ctx, tt.inputLimit, tt.inputOffset)

			require.NoError(t, err)
			assert.Equal(t, expected, got)
		})
	}
}

// ═══════════════════════════════════════════════════════════════════════════
// NOTES FOR TEST WRITING
// ═══════════════════════════════════════════════════════════════════════════
//
// Best Practices:
// 1. Use gomock for mocking repository interfaces
// 2. Use testify/require for assertions that should stop the test
// 3. Use testify/assert for assertions that should continue
// 4. Test both success and error cases
// 5. Test edge cases (nil, empty, invalid input)
// 6. Use table-driven tests for multiple similar cases
// 7. Name tests clearly: TestServiceName_MethodName_Scenario
// 8. Keep tests focused (one behavior per test)
// 9. Use setup/teardown with gomock.Controller
// 10. Verify error wrapping with assert.ErrorIs or assert.Contains
//
// Integration Tests:
// - Put in separate files: *_integration_test.go
// - Use testutil.WithAutoDB for real database
// - Test actual database operations
// - Verify transactions and rollbacks
// - Test concurrent operations if relevant
//
// Workflow Tests:
// - Put in separate files: *_workflow_integration_test.go
// - Test complete workflows across multiple services
// - Use real database and minimal mocking
// - Verify end-to-end behavior
