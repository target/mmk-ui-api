package service

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"
)

const testSiteID = "site-123"

// newSiteService creates a mock repositories and service for testing.
func newSiteService(t *testing.T) (*mocks.MockSiteRepository, *mocks.MockScheduledJobsAdminRepository, *SiteService) {
	t.Helper()
	ctrl := gomock.NewController(t)
	t.Cleanup(ctrl.Finish)

	siteRepo := mocks.NewMockSiteRepository(ctrl)
	adminRepo := mocks.NewMockScheduledJobsAdminRepository(ctrl)

	service := NewSiteService(SiteServiceOptions{
		SiteRepo: siteRepo,
		Admin:    adminRepo,
	})

	return siteRepo, adminRepo, service
}

func TestSiteService_Create_Success(t *testing.T) {
	t.Parallel()
	siteRepo, adminRepo, service := newSiteService(t)

	ctx := context.Background()
	req := &model.CreateSiteRequest{
		Name:            "test-site",
		Enabled:         boolPtr(true),
		Scope:           stringPtr("*"),
		RunEveryMinutes: 15,
		SourceID:        "source-123",
	}

	expectedSite := &model.Site{
		ID:              testSiteID,
		Name:            "test-site",
		Enabled:         true,
		Scope:           stringPtr("*"),
		RunEveryMinutes: 15,
		SourceID:        "source-123",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Mock site creation
	siteRepo.EXPECT().
		Create(ctx, req).
		Return(expectedSite, nil).
		Times(1)

	// Mock schedule reconciliation for enabled site
	expectedPayload := struct {
		SiteID   string `json:"site_id"`
		SourceID string `json:"source_id,omitempty"`
	}{SiteID: testSiteID, SourceID: "source-123"}
	payloadBytes, _ := json.Marshal(expectedPayload)

	adminRepo.EXPECT().
		UpsertByTaskName(ctx, domain.UpsertTaskParams{
			TaskName: "site:" + testSiteID,
			Payload:  payloadBytes,
			Interval: 15 * time.Minute,
		}).
		Return(nil).
		Times(1)

	// Execute
	result, err := service.Create(ctx, req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedSite, result)
}

func TestSiteService_Create_DisabledSite(t *testing.T) {
	t.Parallel()
	siteRepo, adminRepo, service := newSiteService(t)

	ctx := context.Background()
	req := &model.CreateSiteRequest{
		Name:            "disabled-site",
		Enabled:         boolPtr(false),
		RunEveryMinutes: 30,
		SourceID:        "source-456",
	}

	expectedSite := &model.Site{
		ID:              testSiteID,
		Name:            "disabled-site",
		Enabled:         false,
		RunEveryMinutes: 30,
		SourceID:        "source-456",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Mock site creation
	siteRepo.EXPECT().
		Create(ctx, req).
		Return(expectedSite, nil).
		Times(1)

	// Mock schedule deletion for disabled site
	adminRepo.EXPECT().
		DeleteByTaskName(ctx, "site:"+testSiteID).
		Return(true, nil).
		Times(1)

	// Execute
	result, err := service.Create(ctx, req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedSite, result)
}

func TestSiteService_Create_SiteRepoError(t *testing.T) {
	t.Parallel()
	siteRepo, _, service := newSiteService(t)

	ctx := context.Background()
	req := &model.CreateSiteRequest{
		Name:            "test-site",
		RunEveryMinutes: 15,
		SourceID:        "source-123",
	}

	// Mock site creation failure
	siteRepo.EXPECT().
		Create(ctx, req).
		Return(nil, errors.New("database error")).
		Times(1)

	// Execute
	result, err := service.Create(ctx, req)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "database error")
	assert.Nil(t, result)
}

func TestSiteService_Create_ScheduleReconcileError(t *testing.T) {
	t.Parallel()
	siteRepo, adminRepo, service := newSiteService(t)

	ctx := context.Background()
	req := &model.CreateSiteRequest{
		Name:            "test-site",
		Enabled:         boolPtr(true),
		RunEveryMinutes: 15,
		SourceID:        "source-123",
	}

	expectedSite := &model.Site{
		ID:              testSiteID,
		Name:            "test-site",
		Enabled:         true,
		RunEveryMinutes: 15,
		SourceID:        "source-123",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Mock site creation success
	siteRepo.EXPECT().
		Create(ctx, req).
		Return(expectedSite, nil).
		Times(1)

	// Mock schedule reconciliation failure
	adminRepo.EXPECT().
		UpsertByTaskName(ctx, gomock.Any()).
		Return(errors.New("schedule error")).
		Times(1)

	// Execute
	result, err := service.Create(ctx, req)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reconcile schedule")
	assert.Contains(t, err.Error(), "schedule error")
	assert.Nil(t, result)
}

func TestSiteService_Update_Success(t *testing.T) {
	t.Parallel()
	siteRepo, adminRepo, service := newSiteService(t)

	ctx := context.Background()
	updateReq := model.UpdateSiteRequest{
		Name:            stringPtr("updated-site"),
		Enabled:         boolPtr(true),
		RunEveryMinutes: intPtr(30),
	}

	updatedSite := &model.Site{
		ID:              testSiteID,
		Name:            "updated-site",
		Enabled:         true,
		RunEveryMinutes: 30,
		SourceID:        "source-123",
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now(),
	}

	// Mock site update
	siteRepo.EXPECT().
		Update(ctx, testSiteID, updateReq).
		Return(updatedSite, nil).
		Times(1)

	// Mock schedule reconciliation for enabled site
	expectedPayload := struct {
		SiteID   string `json:"site_id"`
		SourceID string `json:"source_id,omitempty"`
	}{SiteID: testSiteID, SourceID: "source-123"}
	payloadBytes, _ := json.Marshal(expectedPayload)

	adminRepo.EXPECT().
		UpsertByTaskName(ctx, domain.UpsertTaskParams{
			TaskName: "site:" + testSiteID,
			Payload:  payloadBytes,
			Interval: 30 * time.Minute,
		}).
		Return(nil).
		Times(1)

	// Execute
	result, err := service.Update(ctx, testSiteID, updateReq)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, updatedSite, result)
}

func TestSiteService_Update_DisableSite(t *testing.T) {
	t.Parallel()
	siteRepo, adminRepo, service := newSiteService(t)

	ctx := context.Background()
	updateReq := model.UpdateSiteRequest{
		Enabled: boolPtr(false),
	}

	updatedSite := &model.Site{
		ID:              testSiteID,
		Name:            "test-site",
		Enabled:         false,
		RunEveryMinutes: 15,
		SourceID:        "source-123",
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now(),
	}

	// Mock site update
	siteRepo.EXPECT().
		Update(ctx, testSiteID, updateReq).
		Return(updatedSite, nil).
		Times(1)

	// Mock schedule deletion for disabled site
	adminRepo.EXPECT().
		DeleteByTaskName(ctx, "site:"+testSiteID).
		Return(true, nil).
		Times(1)

	// Execute
	result, err := service.Update(ctx, testSiteID, updateReq)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, updatedSite, result)
}

func TestSiteService_Update_SiteRepoError(t *testing.T) {
	t.Parallel()
	siteRepo, _, service := newSiteService(t)

	ctx := context.Background()
	updateReq := model.UpdateSiteRequest{
		Name: stringPtr("updated-site"),
	}

	// Mock site update failure
	siteRepo.EXPECT().
		Update(ctx, testSiteID, updateReq).
		Return(nil, errors.New("update failed")).
		Times(1)

	// Execute
	result, err := service.Update(ctx, testSiteID, updateReq)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "update failed")
	assert.Nil(t, result)
}

func TestSiteService_Update_ScheduleReconcileError(t *testing.T) {
	t.Parallel()
	siteRepo, adminRepo, service := newSiteService(t)

	ctx := context.Background()
	updateReq := model.UpdateSiteRequest{
		Enabled: boolPtr(true),
	}

	updatedSite := &model.Site{
		ID:              testSiteID,
		Name:            "test-site",
		Enabled:         true,
		RunEveryMinutes: 15,
		SourceID:        "source-123",
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now(),
	}

	// Mock site update success
	siteRepo.EXPECT().
		Update(ctx, testSiteID, updateReq).
		Return(updatedSite, nil).
		Times(1)

	// Mock schedule reconciliation failure
	adminRepo.EXPECT().
		UpsertByTaskName(ctx, gomock.Any()).
		Return(errors.New("schedule update failed")).
		Times(1)

	// Execute
	result, err := service.Update(ctx, testSiteID, updateReq)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "reconcile schedule")
	assert.Contains(t, err.Error(), "schedule update failed")
	assert.Nil(t, result)
}

func TestSiteService_Update_NoFieldChanges_ReconcilesSchedule(t *testing.T) {
	t.Parallel()
	siteRepo, adminRepo, service := newSiteService(t)

	ctx := context.Background()
	updateReq := model.UpdateSiteRequest{}

	// Repo returns the current site unchanged (enabled with interval)
	unchanged := &model.Site{
		ID:              testSiteID,
		Name:            "test-site",
		Enabled:         true,
		RunEveryMinutes: 10,
		SourceID:        "source-xyz",
		CreatedAt:       time.Now().Add(-time.Hour),
		UpdatedAt:       time.Now(),
	}

	siteRepo.EXPECT().
		Update(ctx, testSiteID, updateReq).
		Return(unchanged, nil).
		Times(1)

	// Expect schedule upsert using the existing values
	expectedPayload := struct {
		SiteID   string `json:"site_id"`
		SourceID string `json:"source_id,omitempty"`
	}{SiteID: testSiteID, SourceID: "source-xyz"}
	payloadBytes, _ := json.Marshal(expectedPayload)

	adminRepo.EXPECT().
		UpsertByTaskName(ctx, domain.UpsertTaskParams{
			TaskName: "site:" + testSiteID,
			Payload:  payloadBytes,
			Interval: 10 * time.Minute,
		}).
		Return(nil).
		Times(1)

	result, err := service.Update(ctx, testSiteID, updateReq)
	require.NoError(t, err)
	assert.Equal(t, unchanged, result)
}

func TestSiteService_Delete_Success(t *testing.T) {
	t.Parallel()
	siteRepo, adminRepo, service := newSiteService(t)

	ctx := context.Background()

	// Mock site deletion success
	siteRepo.EXPECT().
		Delete(ctx, testSiteID).
		Return(true, nil).
		Times(1)

	// Mock schedule deletion
	adminRepo.EXPECT().
		DeleteByTaskName(ctx, "site:"+testSiteID).
		Return(true, nil).
		Times(1)

	// Execute
	result, err := service.Delete(ctx, testSiteID)

	// Assert
	require.NoError(t, err)
	assert.True(t, result)
}

func TestSiteService_Delete_SiteNotFound(t *testing.T) {
	t.Parallel()
	siteRepo, _, service := newSiteService(t)

	ctx := context.Background()

	// Mock site deletion - not found
	siteRepo.EXPECT().
		Delete(ctx, testSiteID).
		Return(false, nil).
		Times(1)

	// Execute
	result, err := service.Delete(ctx, testSiteID)

	// Assert
	require.NoError(t, err)
	assert.False(t, result)
}

func TestSiteService_Delete_SiteRepoError(t *testing.T) {
	t.Parallel()
	siteRepo, _, service := newSiteService(t)

	ctx := context.Background()

	// Mock site deletion failure
	siteRepo.EXPECT().
		Delete(ctx, testSiteID).
		Return(false, errors.New("delete failed")).
		Times(1)

	// Execute
	result, err := service.Delete(ctx, testSiteID)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
	assert.False(t, result)
}

func TestSiteService_Delete_ScheduleDeleteError(t *testing.T) {
	t.Parallel()
	siteRepo, adminRepo, service := newSiteService(t)

	ctx := context.Background()

	// Mock site deletion success
	siteRepo.EXPECT().
		Delete(ctx, testSiteID).
		Return(true, nil).
		Times(1)

	// Mock schedule deletion failure
	adminRepo.EXPECT().
		DeleteByTaskName(ctx, "site:"+testSiteID).
		Return(false, errors.New("schedule delete failed")).
		Times(1)

	// Execute
	result, err := service.Delete(ctx, testSiteID)

	// Assert
	require.Error(t, err)
	assert.Contains(t, err.Error(), "delete schedule")
	assert.Contains(t, err.Error(), "schedule delete failed")
	assert.True(t, result) // Site was deleted successfully
}

func TestSiteService_ReconcileSchedule_NilAdmin(t *testing.T) {
	t.Parallel()
	siteRepo, _, _ := newSiteService(t)

	// Create service with nil admin repo
	service := NewSiteService(SiteServiceOptions{
		SiteRepo: siteRepo,
		Admin:    nil,
	})

	ctx := context.Background()
	req := &model.CreateSiteRequest{
		Name:            "test-site",
		Enabled:         boolPtr(true),
		RunEveryMinutes: 15,
		SourceID:        "source-123",
	}

	expectedSite := &model.Site{
		ID:              testSiteID,
		Name:            "test-site",
		Enabled:         true,
		RunEveryMinutes: 15,
		SourceID:        "source-123",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Mock site creation
	siteRepo.EXPECT().
		Create(ctx, req).
		Return(expectedSite, nil).
		Times(1)

	// Execute - should not fail even with nil admin repo
	result, err := service.Create(ctx, req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedSite, result)
}

func TestSiteService_ReconcileSchedule_ZeroInterval(t *testing.T) {
	t.Parallel()
	siteRepo, adminRepo, service := newSiteService(t)

	ctx := context.Background()
	req := &model.CreateSiteRequest{
		Name:            "test-site",
		Enabled:         boolPtr(true),
		RunEveryMinutes: 0, // Zero interval should default to 1 minute
		SourceID:        "source-123",
	}

	expectedSite := &model.Site{
		ID:              testSiteID,
		Name:            "test-site",
		Enabled:         true,
		RunEveryMinutes: 0,
		SourceID:        "source-123",
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	// Mock site creation
	siteRepo.EXPECT().
		Create(ctx, req).
		Return(expectedSite, nil).
		Times(1)

	// Mock schedule reconciliation - should use 1 minute interval
	expectedPayload := struct {
		SiteID   string `json:"site_id"`
		SourceID string `json:"source_id,omitempty"`
	}{SiteID: testSiteID, SourceID: "source-123"}
	payloadBytes, _ := json.Marshal(expectedPayload)

	adminRepo.EXPECT().
		UpsertByTaskName(ctx, domain.UpsertTaskParams{
			TaskName: "site:" + testSiteID,
			Payload:  payloadBytes,
			Interval: time.Minute, // Should default to 1 minute
		}).
		Return(nil).
		Times(1)

	// Execute
	result, err := service.Create(ctx, req)

	// Assert
	require.NoError(t, err)
	assert.Equal(t, expectedSite, result)
}

func TestTaskNameForSite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		siteID string
		want   string
	}{
		{
			name:   "normal site ID",
			siteID: "site-123",
			want:   "site:site-123",
		},
		{
			name:   "UUID site ID",
			siteID: "550e8400-e29b-41d4-a716-446655440000",
			want:   "site:550e8400-e29b-41d4-a716-446655440000",
		},
		{
			name:   "empty site ID",
			siteID: "",
			want:   "site:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := taskNameForSite(tt.siteID)
			assert.Equal(t, tt.want, result)
		})
	}
}

// Helper functions.
func boolPtr(b bool) *bool       { return &b }
func stringPtr(s string) *string { return &s }
func intPtr(i int) *int          { return &i }
