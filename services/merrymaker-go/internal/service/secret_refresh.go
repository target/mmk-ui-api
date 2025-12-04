package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// SecretRefreshServiceOptions groups dependencies for SecretRefreshService.
type SecretRefreshServiceOptions struct {
	SecretRepo core.SecretRepository
	AdminRepo  core.ScheduledJobsAdminRepository
	JobRepo    core.JobRepository
	Logger     *slog.Logger
	DebugMode  bool
}

// SecretRefreshService orchestrates secret refresh operations for dynamic secrets.
type SecretRefreshService struct {
	secretRepo core.SecretRepository
	adminRepo  core.ScheduledJobsAdminRepository
	jobRepo    core.JobRepository
	logger     *slog.Logger
	debugMode  bool
}

// NewSecretRefreshService constructs a new SecretRefreshService.
func NewSecretRefreshService(opts SecretRefreshServiceOptions) (*SecretRefreshService, error) {
	if opts.SecretRepo == nil {
		return nil, errors.New("SecretRepo is required")
	}
	if opts.AdminRepo == nil {
		return nil, errors.New("AdminRepo is required")
	}

	var logger *slog.Logger
	if opts.Logger != nil {
		logger = opts.Logger.With("component", "secret_refresh_service")
	}

	svc := &SecretRefreshService{
		secretRepo: opts.SecretRepo,
		adminRepo:  opts.AdminRepo,
		jobRepo:    opts.JobRepo,
		logger:     logger,
		debugMode:  opts.DebugMode,
	}

	// Log if debug mode is enabled (logs actual secret values)
	if svc.debugMode && logger != nil {
		logger.Info("secret refresh debug mode enabled - actual secret values will be logged",
			"debug_mode", true,
			"security_warning", "actual secret values will appear in logs",
			"recommendation", "disable in production")
	}

	return svc, nil
}

// MustNewSecretRefreshService constructs a new SecretRefreshService and panics on error.
func MustNewSecretRefreshService(opts SecretRefreshServiceOptions) *SecretRefreshService {
	svc, err := NewSecretRefreshService(opts)
	if err != nil {
		panic(err) //nolint:forbidigo // Must constructor fails fast when dependencies are invalid during startup
	}
	return svc
}

// ReconcileSchedule creates or deletes the scheduled job for a secret based on its refresh_enabled state.
// This should be called after creating or updating a secret with refresh configuration.
func (s *SecretRefreshService) ReconcileSchedule(ctx context.Context, secret *model.Secret) error {
	if secret == nil {
		return errors.New("secret is nil")
	}

	taskName := taskNameForSecretRefresh(secret.ID)

	if !secret.RefreshEnabled {
		// Refresh disabled - remove scheduled job if it exists
		return s.RemoveSchedule(ctx, secret.ID)
	}

	if err := ensureRefreshConfig(secret); err != nil {
		return err
	}

	interval := time.Duration(*secret.RefreshInterval) * time.Second

	payload := struct {
		SecretID string `json:"secret_id"`
	}{SecretID: secret.ID}

	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}

	policy := domain.OverrunPolicySkip
	states := domain.OverrunStateRunning | domain.OverrunStatePending | domain.OverrunStateRetrying
	return s.adminRepo.UpsertByTaskName(ctx, domain.UpsertTaskParams{
		TaskName:      taskName,
		Payload:       b,
		Interval:      interval,
		OverrunPolicy: &policy,
		OverrunStates: &states,
	})
}

// RemoveSchedule removes the scheduled refresh job for a secret and cleans up any pending jobs.
func (s *SecretRefreshService) RemoveSchedule(ctx context.Context, secretID string) error {
	taskName := taskNameForSecretRefresh(secretID)

	// Remove the scheduled job
	_, err := s.adminRepo.DeleteByTaskName(ctx, taskName)
	if err != nil {
		return err
	}

	// Clean up any pending jobs for this secret (completed/failed jobs are retained for audit history)
	if s.jobRepo == nil {
		return nil
	}

	_, delErr := s.jobRepo.DeleteByPayloadField(ctx, core.DeleteByPayloadFieldParams{
		JobType:    model.JobTypeSecretRefresh,
		FieldName:  "secret_id",
		FieldValue: secretID,
	})
	if delErr != nil {
		s.logDeleteError(ctx, secretID, delErr)
	}

	return nil
}

// ExecuteRefresh executes a secret refresh by running the provider script.
// This is called by the job runner when a refresh job is processed.
func (s *SecretRefreshService) ExecuteRefresh(ctx context.Context, secretID string) error {
	secret, err := s.secretRepo.GetByID(ctx, secretID)
	if err != nil {
		return fmt.Errorf("get secret: %w", err)
	}

	if validateErr := s.validateRefreshConfig(secret); validateErr != nil {
		return validateErr
	}

	// Execute provider script
	newValue, err := s.ExecuteProviderScript(ctx, secret)
	if err != nil {
		s.markRefreshFailed(ctx, secretID, err)
		return fmt.Errorf("execute provider script: %w", err)
	}

	s.logSecretValueIfDebug(ctx, secretID, secret.Name, newValue)

	// Update secret value
	if updateErr := s.updateSecretValue(ctx, secretID, newValue); updateErr != nil {
		s.markRefreshFailed(ctx, secretID, updateErr)
		return fmt.Errorf("update secret value: %w", updateErr)
	}

	// Update status to success
	if successErr := s.markRefreshSuccess(ctx, secretID); successErr != nil {
		return fmt.Errorf("update refresh status: %w", successErr)
	}

	s.logRefreshSuccess(ctx, secretID, secret.Name)
	return nil
}

// validateRefreshConfig validates that the secret has the required refresh configuration.
func (s *SecretRefreshService) validateRefreshConfig(secret *model.Secret) error {
	if !secret.RefreshEnabled {
		return errors.New("secret refresh is not enabled")
	}
	if secret.ProviderScriptPath == nil || *secret.ProviderScriptPath == "" {
		return errors.New("provider_script_path is not configured")
	}
	return nil
}

// logSecretValueIfDebug logs a redacted preview of the secret value if debug mode is enabled.
// Shows first 8 chars + length to aid debugging without exposing full secret in persistent logs.
func (s *SecretRefreshService) logSecretValueIfDebug(ctx context.Context, secretID, secretName, value string) {
	if s.debugMode && s.logger != nil {
		// Show redacted preview to avoid leaking full secrets into persistent logs/APMs
		previewLen := min(8, len(value))
		preview := value[:previewLen]
		if len(value) > previewLen {
			preview += "..."
		}

		s.logger.WarnContext(ctx, "secret value resolved from provider script (DEBUG MODE)",
			"secret_id", secretID,
			"secret_name", secretName,
			"value_preview", preview,
			"value_length", len(value),
			"security_warning", "debug mode enabled - showing redacted preview only",
			"recommendation", "disable debug mode in production")
	}
}

func ensureRefreshConfig(secret *model.Secret) error {
	if secret.ProviderScriptPath == nil || *secret.ProviderScriptPath == "" {
		return errors.New("provider_script_path is required for refresh-enabled secrets")
	}
	if secret.RefreshInterval == nil || *secret.RefreshInterval <= 0 {
		return errors.New("refresh_interval is required for refresh-enabled secrets")
	}
	return nil
}

func (s *SecretRefreshService) logDeleteError(ctx context.Context, secretID string, err error) {
	if s.logger == nil {
		return
	}

	s.logger.WarnContext(ctx, "failed to delete queued secret refresh jobs",
		"secret_id", secretID,
		"error", err)
}

// logRefreshSuccess logs a successful secret refresh.
func (s *SecretRefreshService) logRefreshSuccess(ctx context.Context, secretID, secretName string) {
	if s.logger != nil {
		s.logger.InfoContext(ctx, "secret refreshed successfully",
			"secret_id", secretID,
			"secret_name", secretName)
	}
}

// markRefreshSuccess updates the refresh status to success.
func (s *SecretRefreshService) markRefreshSuccess(ctx context.Context, secretID string) error {
	return s.secretRepo.UpdateRefreshStatus(ctx, core.UpdateSecretRefreshStatusParams{
		SecretID:    secretID,
		Status:      "success",
		ErrorMsg:    nil,
		RefreshedAt: time.Now(),
	})
}

// markRefreshFailed updates the refresh status to failed.
func (s *SecretRefreshService) markRefreshFailed(ctx context.Context, secretID string, err error) {
	errMsg := err.Error()
	if updateErr := s.secretRepo.UpdateRefreshStatus(ctx, core.UpdateSecretRefreshStatusParams{
		SecretID:    secretID,
		Status:      "failed",
		ErrorMsg:    &errMsg,
		RefreshedAt: time.Now(),
	}); updateErr != nil && s.logger != nil {
		s.logger.ErrorContext(ctx, "failed to update secret refresh status", "secret_id", secretID, "error", updateErr)
	}
}

// updateSecretValue updates the secret value.
func (s *SecretRefreshService) updateSecretValue(ctx context.Context, secretID, newValue string) error {
	updateReq := model.UpdateSecretRequest{
		Value: &newValue,
	}
	_, err := s.secretRepo.Update(ctx, secretID, updateReq)
	return err
}

// ExecuteProviderScript runs the provider script and returns the new secret value.
// This is a public method that can be used by other services (e.g., SecretService for initial value population).
func (s *SecretRefreshService) ExecuteProviderScript(ctx context.Context, secret *model.Secret) (string, error) {
	if secret.ProviderScriptPath == nil {
		return "", errors.New("provider_script_path is nil")
	}

	// Parse env config
	var envMap map[string]string
	if len(secret.EnvConfig) > 0 {
		if err := json.Unmarshal(secret.EnvConfig, &envMap); err != nil {
			return "", fmt.Errorf("parse env config: %w", err)
		}
	}

	// Build environment variables (append to base environment to preserve PATH, etc.)
	env := make([]string, 0, len(envMap))
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}

	// Execute script
	// #nosec G204 -- provider_script_path is admin-configured and stored in DB, not user input
	cmd := exec.CommandContext(ctx, *secret.ProviderScriptPath)
	cmd.Env = append(os.Environ(), env...)

	output, err := cmd.Output()
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			stderr := string(exitErr.Stderr)
			return "", fmt.Errorf("script failed (exit %d): %s", exitErr.ExitCode(), stderr)
		}
		return "", fmt.Errorf("execute script: %w", err)
	}

	// Parse output (single line)
	newValue := strings.TrimSpace(string(output))
	if newValue == "" {
		return "", errors.New("script returned empty value")
	}

	return newValue, nil
}

// FindDueForRefresh finds secrets that need to be refreshed.
// This can be used by a background worker to process refresh jobs.
func (s *SecretRefreshService) FindDueForRefresh(ctx context.Context, limit int) ([]*model.Secret, error) {
	return s.secretRepo.FindDueForRefresh(ctx, limit)
}

func taskNameForSecretRefresh(secretID string) string {
	return "secret-refresh:" + secretID
}
