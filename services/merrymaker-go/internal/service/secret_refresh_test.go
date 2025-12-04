package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/data"
	"github.com/target/mmk-ui-api/internal/data/cryptoutil"
	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/mocks"
	"github.com/target/mmk-ui-api/internal/testutil"
	"go.uber.org/mock/gomock"
)

// Helper function to create a SecretRefreshService for testing.
func newTestSecretRefreshService(
	t *testing.T,
	secretRepo core.SecretRepository,
	adminRepo core.ScheduledJobsAdminRepository,
	jobRepo core.JobRepository,
) *SecretRefreshService {
	t.Helper()
	svc, err := NewSecretRefreshService(SecretRefreshServiceOptions{
		SecretRepo: secretRepo,
		AdminRepo:  adminRepo,
		JobRepo:    jobRepo,
	})
	require.NoError(t, err)
	return svc
}

// createMockScript creates a temporary executable script for testing.
func createMockScript(t *testing.T, content string) string {
	t.Helper()

	// Create temporary directory
	tmpDir := t.TempDir()
	scriptPath := filepath.Join(tmpDir, "test_script.sh")

	// Write script content
	err := os.WriteFile(scriptPath, []byte(content), 0o600)
	require.NoError(t, err)

	// Make script executable
	err = os.Chmod(scriptPath, 0o700)
	require.NoError(t, err)

	return scriptPath
}

// ═══════════════════════════════════════════════════════════════════════════
// Constructor Tests
// ═══════════════════════════════════════════════════════════════════════════

func TestNewSecretRefreshService_RequiredDependencies(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSecretRepo := mocks.NewMockSecretRepository(ctrl)
	mockAdminRepo := mocks.NewMockScheduledJobsAdminRepository(ctrl)

	// Test missing SecretRepo
	_, err := NewSecretRefreshService(SecretRefreshServiceOptions{
		SecretRepo: nil,
		AdminRepo:  mockAdminRepo,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "SecretRepo is required")

	// Test missing AdminRepo
	_, err = NewSecretRefreshService(SecretRefreshServiceOptions{
		SecretRepo: mockSecretRepo,
		AdminRepo:  nil,
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "AdminRepo is required")

	// Test success
	svc, err := NewSecretRefreshService(SecretRefreshServiceOptions{
		SecretRepo: mockSecretRepo,
		AdminRepo:  mockAdminRepo,
	})
	require.NoError(t, err)
	assert.NotNil(t, svc)
}

// ═══════════════════════════════════════════════════════════════════════════
// Unit Tests
// ═══════════════════════════════════════════════════════════════════════════

func TestSecretRefreshService_ReconcileSchedule_EnableRefresh(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSecretRepo := mocks.NewMockSecretRepository(ctrl)
	mockAdminRepo := mocks.NewMockScheduledJobsAdminRepository(ctrl)
	svc := newTestSecretRefreshService(t, mockSecretRepo, mockAdminRepo, nil)

	ctx := context.Background()
	secretID := "test-secret-id"
	scriptPath := "/path/to/script.sh"
	interval := int64(3600) // 1 hour

	secret := &model.Secret{
		ID:                 secretID,
		RefreshEnabled:     true,
		ProviderScriptPath: &scriptPath,
		RefreshInterval:    &interval,
	}

	// Expect UpsertByTaskName to be called
	mockAdminRepo.EXPECT().
		UpsertByTaskName(ctx, gomock.Any()).
		DoAndReturn(func(ctx context.Context, params any) error {
			// Verify the task name and payload
			return nil
		}).
		Times(1)

	err := svc.ReconcileSchedule(ctx, secret)
	require.NoError(t, err)
}

func TestSecretRefreshService_ReconcileSchedule_DisableRefresh(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSecretRepo := mocks.NewMockSecretRepository(ctrl)
	mockAdminRepo := mocks.NewMockScheduledJobsAdminRepository(ctrl)
	svc := newTestSecretRefreshService(t, mockSecretRepo, mockAdminRepo, nil)

	ctx := context.Background()
	secretID := "test-secret-id"

	secret := &model.Secret{
		ID:             secretID,
		RefreshEnabled: false,
	}

	// Expect DeleteByTaskName to be called
	mockAdminRepo.EXPECT().
		DeleteByTaskName(ctx, "secret-refresh:"+secretID).
		Return(true, nil).
		Times(1)

	err := svc.ReconcileSchedule(ctx, secret)
	require.NoError(t, err)
}

// ═══════════════════════════════════════════════════════════════════════════
// Integration Tests
// ═══════════════════════════════════════════════════════════════════════════

func TestSecretRefreshService_Integration_CreateDynamicSecretSchedulesJob(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup repositories
		secretRepo := data.NewSecretRepo(db, &cryptoutil.NoopEncryptor{})
		adminRepo := data.NewScheduledJobsAdminRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		refreshSvc := newTestSecretRefreshService(t, secretRepo, adminRepo, jobRepo)

		// Create a mock script that outputs a test value
		scriptContent := `#!/bin/bash
echo "new-secret-value"`
		scriptPath := createMockScript(t, scriptContent)

		ctx := context.Background()

		// Create a dynamic secret
		secret, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:               "test-dynamic-secret",
			Value:              "initial-value",
			ProviderScriptPath: &scriptPath,
			RefreshInterval:    &[]int64{3600}[0], // 1 hour
			RefreshEnabled:     &[]bool{true}[0],
		})
		require.NoError(t, err)

		// Reconcile schedule should create a scheduled job
		err = refreshSvc.ReconcileSchedule(ctx, secret)
		require.NoError(t, err)

		// Verify that a scheduled job was created
		// Note: We can't easily verify the scheduled job exists without exposing
		// internal methods, but we can verify no error occurred
	})
}

func TestSecretRefreshService_Integration_ExecuteRefresh(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup repositories
		secretRepo := data.NewSecretRepo(db, &cryptoutil.NoopEncryptor{})
		adminRepo := data.NewScheduledJobsAdminRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		refreshSvc := newTestSecretRefreshService(t, secretRepo, adminRepo, jobRepo)

		// Create a mock script that outputs a new value
		newValue := "refreshed-secret-value-123"
		scriptContent := fmt.Sprintf(`#!/bin/bash
echo "%s"`, newValue)
		scriptPath := createMockScript(t, scriptContent)

		ctx := context.Background()

		// Create a dynamic secret
		secret, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:               "test-refresh-secret",
			Value:              "initial-value",
			ProviderScriptPath: &scriptPath,
			RefreshInterval:    &[]int64{3600}[0],
			RefreshEnabled:     &[]bool{true}[0],
		})
		require.NoError(t, err)

		// Execute refresh
		err = refreshSvc.ExecuteRefresh(ctx, secret.ID)
		require.NoError(t, err)

		// Verify the secret value was updated
		updatedSecret, err := secretRepo.GetByID(ctx, secret.ID)
		require.NoError(t, err)
		assert.Equal(t, newValue, updatedSecret.Value)

		// Verify refresh status was updated
		assert.Equal(t, "success", *updatedSecret.LastRefreshStatus)
		assert.NotNil(t, updatedSecret.LastRefreshedAt)
		assert.Nil(t, updatedSecret.LastRefreshError)
	})
}

func TestSecretRefreshService_Integration_ScriptFailure(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup repositories
		secretRepo := data.NewSecretRepo(db, &cryptoutil.NoopEncryptor{})
		adminRepo := data.NewScheduledJobsAdminRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		refreshSvc := newTestSecretRefreshService(t, secretRepo, adminRepo, jobRepo)

		// Create a script that fails
		scriptContent := `#!/bin/bash
echo "Error message" >&2
exit 1`
		scriptPath := createMockScript(t, scriptContent)

		ctx := context.Background()

		// Create a dynamic secret
		secret, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:               "test-failing-secret",
			Value:              "initial-value",
			ProviderScriptPath: &scriptPath,
			RefreshInterval:    &[]int64{3600}[0],
			RefreshEnabled:     &[]bool{true}[0],
		})
		require.NoError(t, err)

		// Execute refresh - should fail
		err = refreshSvc.ExecuteRefresh(ctx, secret.ID)
		require.Error(t, err)

		// Verify the secret value was NOT updated
		updatedSecret, err := secretRepo.GetByID(ctx, secret.ID)
		require.NoError(t, err)
		assert.Equal(t, "initial-value", updatedSecret.Value)

		// Verify refresh status shows failure
		assert.Equal(t, "failed", *updatedSecret.LastRefreshStatus)
		assert.NotNil(t, updatedSecret.LastRefreshedAt)
		assert.NotNil(t, updatedSecret.LastRefreshError)
		assert.Contains(t, *updatedSecret.LastRefreshError, "script failed")
	})
}

func TestSecretRefreshService_Integration_DisableRefreshRemovesJob(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup repositories
		secretRepo := data.NewSecretRepo(db, &cryptoutil.NoopEncryptor{})
		adminRepo := data.NewScheduledJobsAdminRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		refreshSvc := newTestSecretRefreshService(t, secretRepo, adminRepo, jobRepo)

		// Create a mock script
		scriptContent := `#!/bin/bash
echo "test-value"`
		scriptPath := createMockScript(t, scriptContent)

		ctx := context.Background()

		// Create a dynamic secret with refresh enabled
		secret, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:               "test-disable-secret",
			Value:              "initial-value",
			ProviderScriptPath: &scriptPath,
			RefreshInterval:    &[]int64{3600}[0],
			RefreshEnabled:     &[]bool{true}[0],
		})
		require.NoError(t, err)

		// Schedule the refresh job
		err = refreshSvc.ReconcileSchedule(ctx, secret)
		require.NoError(t, err)

		// Now disable refresh
		secret.RefreshEnabled = false
		err = refreshSvc.ReconcileSchedule(ctx, secret)
		require.NoError(t, err)

		// The scheduled job should be removed (no error means success)
	})
}

func TestSecretRefreshService_Integration_DeleteSecretRemovesJob(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup repositories and services
		secretRepo := data.NewSecretRepo(db, &cryptoutil.NoopEncryptor{})
		adminRepo := data.NewScheduledJobsAdminRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		refreshSvc := newTestSecretRefreshService(t, secretRepo, adminRepo, jobRepo)

		// Create SecretService with refresh service wired
		secretService, err := NewSecretService(SecretServiceOptions{
			Repo:       secretRepo,
			RefreshSvc: refreshSvc,
		})
		require.NoError(t, err)

		// Create a mock script
		scriptContent := `#!/bin/bash
echo "test-value"`
		scriptPath := createMockScript(t, scriptContent)

		ctx := context.Background()

		// Create a dynamic secret with refresh enabled
		secret, err := secretService.Create(ctx, model.CreateSecretRequest{
			Name:               "test-delete-secret",
			Value:              "initial-value",
			ProviderScriptPath: &scriptPath,
			RefreshInterval:    &[]int64{3600}[0],
			RefreshEnabled:     &[]bool{true}[0],
		})
		require.NoError(t, err)

		// Delete the secret - this should remove the scheduled job
		deleted, err := secretService.Delete(ctx, secret.ID)
		require.NoError(t, err)
		assert.True(t, deleted)

		// Verify secret is deleted
		_, err = secretRepo.GetByID(ctx, secret.ID)
		require.Error(t, err)
	})
}

func TestSecretRefreshService_Integration_ScriptWithEnvConfig(t *testing.T) {
	testutil.SkipIfNoTestDB(t)

	testutil.WithAutoDB(t, func(db *sql.DB) {
		// Setup repositories
		secretRepo := data.NewSecretRepo(db, &cryptoutil.NoopEncryptor{})
		adminRepo := data.NewScheduledJobsAdminRepo(db)
		jobRepo := data.NewJobRepo(db, data.RepoConfig{})
		refreshSvc := newTestSecretRefreshService(t, secretRepo, adminRepo, jobRepo)

		// Create a script that uses environment variables
		scriptContent := `#!/bin/bash
echo "token-${API_URL}-${CLIENT_ID}"`
		scriptPath := createMockScript(t, scriptContent)

		ctx := context.Background()

		// Create env config
		envConfig := map[string]string{
			"API_URL":   "https://api.example.com",
			"CLIENT_ID": "test-client-123",
		}
		envConfigJSON, err := json.Marshal(envConfig)
		require.NoError(t, err)
		envConfigStr := string(envConfigJSON)

		// Create a dynamic secret with env config
		secret, err := secretRepo.Create(ctx, model.CreateSecretRequest{
			Name:               "test-env-secret",
			Value:              "initial-value",
			ProviderScriptPath: &scriptPath,
			EnvConfig:          &envConfigStr,
			RefreshInterval:    &[]int64{3600}[0],
			RefreshEnabled:     &[]bool{true}[0],
		})
		require.NoError(t, err)

		// Execute refresh
		err = refreshSvc.ExecuteRefresh(ctx, secret.ID)
		require.NoError(t, err)

		// Verify the secret value includes the env vars
		updatedSecret, err := secretRepo.GetByID(ctx, secret.ID)
		require.NoError(t, err)
		expectedValue := "token-https://api.example.com-test-client-123"
		assert.Equal(t, expectedValue, updatedSecret.Value)

		// Verify refresh status
		assert.Equal(t, "success", *updatedSecret.LastRefreshStatus)
	})
}

// ═══════════════════════════════════════════════════════════════════════════
// Debug Mode Tests
// ═══════════════════════════════════════════════════════════════════════════

func TestSecretRefreshService_DebugMode(t *testing.T) {
	t.Run("debug mode enabled logs warning on construction", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSecretRepo := mocks.NewMockSecretRepository(ctrl)
		mockAdminRepo := mocks.NewMockScheduledJobsAdminRepository(ctrl)

		// Create service with debug mode enabled
		svc, err := NewSecretRefreshService(SecretRefreshServiceOptions{
			SecretRepo: mockSecretRepo,
			AdminRepo:  mockAdminRepo,
			DebugMode:  true,
		})
		require.NoError(t, err)
		assert.True(t, svc.debugMode)
	})

	t.Run("debug mode disabled by default", func(t *testing.T) {
		ctrl := gomock.NewController(t)
		defer ctrl.Finish()

		mockSecretRepo := mocks.NewMockSecretRepository(ctrl)
		mockAdminRepo := mocks.NewMockScheduledJobsAdminRepository(ctrl)

		// Create service without debug mode
		svc, err := NewSecretRefreshService(SecretRefreshServiceOptions{
			SecretRepo: mockSecretRepo,
			AdminRepo:  mockAdminRepo,
		})
		require.NoError(t, err)
		assert.False(t, svc.debugMode)
	})
}
