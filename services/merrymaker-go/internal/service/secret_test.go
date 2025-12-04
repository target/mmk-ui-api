package service

import (
	"context"
	"errors"
	"testing"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const testSecretID = "test-id"

// Helper function to create a SecretService for testing.
func newTestSecretService(t *testing.T, repo core.SecretRepository) *SecretService {
	t.Helper()
	svc, err := NewSecretService(SecretServiceOptions{Repo: repo})
	require.NoError(t, err)
	return svc
}

func TestNewSecretService_RequiredDependency(t *testing.T) {
	// Test that constructor returns error when required dependency is nil
	svc, err := NewSecretService(SecretServiceOptions{
		Repo: nil, // Required dependency is nil
	})

	require.Error(t, err)
	assert.Nil(t, svc)
	assert.Contains(t, err.Error(), "SecretRepository is required")
}

func TestNewSecretService_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSecretRepository(ctrl)

	// Test that constructor succeeds with valid dependencies
	svc, err := NewSecretService(SecretServiceOptions{
		Repo: mockRepo,
	})

	require.NoError(t, err)
	assert.NotNil(t, svc)
}

func TestMustNewSecretService_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSecretRepository(ctrl)

	// Test that MustNewSecretService succeeds with valid dependencies
	svc := MustNewSecretService(SecretServiceOptions{
		Repo: mockRepo,
	})

	assert.NotNil(t, svc)
}

func TestMustNewSecretService_Panics(t *testing.T) {
	// Test that MustNewSecretService panics when required dependency is nil
	assert.Panics(t, func() {
		MustNewSecretService(SecretServiceOptions{
			Repo: nil, // Required dependency is nil
		})
	})
}

func TestSecretService_Create_Success(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSecretRepository(ctrl)
	svc := newTestSecretService(t, mockRepo)

	ctx := context.Background()
	req := model.CreateSecretRequest{
		Name:  "TEST_SECRET",
		Value: "secret-value-123",
	}
	expected := &model.Secret{
		ID:    testSecretID,
		Name:  "TEST_SECRET",
		Value: "secret-value-123",
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

func TestSecretService_Create_RepositoryError(t *testing.T) {
	// Setup
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSecretRepository(ctrl)
	svc := newTestSecretService(t, mockRepo)

	ctx := context.Background()
	req := model.CreateSecretRequest{Name: "test"}
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
	assert.Contains(t, err.Error(), "create secret")
	assert.ErrorIs(t, err, repoErr)
}

func TestSecretService_GetByID_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSecretRepository(ctrl)
	svc := newTestSecretService(t, mockRepo)

	ctx := context.Background()
	id := testSecretID
	expected := &model.Secret{ID: id, Name: "test"}

	mockRepo.EXPECT().
		GetByID(ctx, id).
		Return(expected, nil)

	got, err := svc.GetByID(ctx, id)

	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestSecretService_GetByName_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSecretRepository(ctrl)
	svc := newTestSecretService(t, mockRepo)

	ctx := context.Background()
	name := "TEST_SECRET"
	expected := &model.Secret{ID: testSecretID, Name: name}

	mockRepo.EXPECT().
		GetByName(ctx, name).
		Return(expected, nil)

	got, err := svc.GetByName(ctx, name)

	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestSecretService_List_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSecretRepository(ctrl)
	svc := newTestSecretService(t, mockRepo)

	ctx := context.Background()
	limit, offset := 10, 0
	expected := []*model.Secret{
		{ID: "1", Name: "secret1"},
		{ID: "2", Name: "secret2"},
	}

	mockRepo.EXPECT().
		List(ctx, limit, offset).
		Return(expected, nil)

	got, err := svc.List(ctx, limit, offset)

	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestSecretService_Update_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSecretRepository(ctrl)
	svc := newTestSecretService(t, mockRepo)

	ctx := context.Background()
	id := testSecretID
	updatedName := "updated"
	req := model.UpdateSecretRequest{Name: &updatedName}
	expected := &model.Secret{ID: id, Name: "updated"}

	// Expect GetByID to check if validation is needed
	mockRepo.EXPECT().
		GetByID(ctx, id).
		Return(&model.Secret{ID: id, Name: "old", RefreshEnabled: false}, nil)

	mockRepo.EXPECT().
		Update(ctx, id, req).
		Return(expected, nil)

	got, err := svc.Update(ctx, id, req)

	require.NoError(t, err)
	assert.Equal(t, expected, got)
}

func TestSecretService_Delete_Success(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockRepo := mocks.NewMockSecretRepository(ctrl)
	svc := newTestSecretService(t, mockRepo)

	ctx := context.Background()
	id := testSecretID

	// Expect GetByID call (to check if refresh is enabled)
	mockRepo.EXPECT().
		GetByID(ctx, id).
		Return(&model.Secret{
			ID:             id,
			Name:           "test-secret",
			RefreshEnabled: false,
		}, nil)

	mockRepo.EXPECT().
		Delete(ctx, id).
		Return(true, nil)

	deleted, err := svc.Delete(ctx, id)

	require.NoError(t, err)
	assert.True(t, deleted)
}
