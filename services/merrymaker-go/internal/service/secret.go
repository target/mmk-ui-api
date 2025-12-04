package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/target/mmk-ui-api/internal/core"
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// SecretServiceOptions groups dependencies for SecretService.
type SecretServiceOptions struct {
	Repo       core.SecretRepository // Required: secret repository
	RefreshSvc *SecretRefreshService // Optional: secret refresh service for dynamic secrets
	Logger     *slog.Logger          // Optional: structured logger
}

// SecretService provides business logic for secret operations.
type SecretService struct {
	repo       core.SecretRepository
	refreshSvc *SecretRefreshService
	logger     *slog.Logger
}

// NewSecretService constructs a new SecretService.
func NewSecretService(opts SecretServiceOptions) (*SecretService, error) {
	if opts.Repo == nil {
		return nil, errors.New("SecretRepository is required")
	}

	// Create component-scoped logger
	var logger *slog.Logger
	if opts.Logger != nil {
		logger = opts.Logger.With("component", "secret_service")
		logger.Debug("SecretService initialized")
	}

	return &SecretService{
		repo:       opts.Repo,
		refreshSvc: opts.RefreshSvc,
		logger:     logger,
	}, nil
}

// MustNewSecretService constructs a new SecretService and panics on error.
// Use this when you want fail-fast behavior during application startup.
func MustNewSecretService(opts SecretServiceOptions) *SecretService {
	service, err := NewSecretService(opts)
	if err != nil {
		panic(err) //nolint:forbidigo // Must constructor fails fast when dependencies are invalid during startup
	}
	return service
}

// parseEnvConfig converts the env config string to json.RawMessage.
func (s *SecretService) parseEnvConfig(envConfig *string) json.RawMessage {
	if envConfig == nil || strings.TrimSpace(*envConfig) == "" {
		return nil
	}
	return json.RawMessage(*envConfig)
}

// Create creates a new secret with the given request parameters.
// If the secret has refresh_enabled=true, it will validate the provider script and schedule the refresh job.
// If refresh is enabled and no value is provided, it will run the refresh script immediately to populate the value.
func (s *SecretService) Create(
	ctx context.Context,
	req model.CreateSecretRequest,
) (*model.Secret, error) {
	// Validate and populate value if refresh is enabled
	if err := s.validateAndPopulateRefreshOnCreate(ctx, &req); err != nil {
		return nil, err
	}

	secret, err := s.repo.Create(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("create secret: %w", err)
	}

	// Schedule refresh job if enabled
	s.scheduleRefreshIfEnabled(ctx, secret)

	// Optional: Log success (avoid logging sensitive names)
	if s.logger != nil && secret != nil {
		s.logger.DebugContext(ctx, "secret created", "id", secret.ID, "refresh_enabled", secret.RefreshEnabled)
	}

	return secret, nil
}

// validateAndPopulateRefreshOnCreate validates the provider script and populates the value if needed.
func (s *SecretService) validateAndPopulateRefreshOnCreate(ctx context.Context, req *model.CreateSecretRequest) error {
	if req.RefreshEnabled == nil || !*req.RefreshEnabled {
		return nil
	}

	if s.refreshSvc == nil {
		return errors.New("refresh service not available")
	}

	// Create a temporary secret object to validate the script
	tempSecret := &model.Secret{
		ProviderScriptPath: req.ProviderScriptPath,
		EnvConfig:          s.parseEnvConfig(req.EnvConfig),
		RefreshEnabled:     true,
	}

	// Execute the provider script to validate it works
	scriptValue, err := s.refreshSvc.ExecuteProviderScript(ctx, tempSecret)
	if err != nil {
		return s.providerScriptError(err, req.ProviderScriptPath)
	}

	// If no value was provided, use the script output as the initial value
	if strings.TrimSpace(req.Value) == "" {
		req.Value = scriptValue
	}

	return nil
}

// scheduleRefreshIfEnabled schedules a refresh job if the secret has refresh enabled.
func (s *SecretService) scheduleRefreshIfEnabled(ctx context.Context, secret *model.Secret) {
	if !secret.RefreshEnabled || s.refreshSvc == nil {
		return
	}

	if err := s.refreshSvc.ReconcileSchedule(ctx, secret); err != nil {
		// Log error but don't fail the operation
		if s.logger != nil {
			s.logger.ErrorContext(ctx, "failed to schedule secret refresh",
				"secret_id", secret.ID,
				"error", err)
		}
	}
}

// GetByID retrieves a secret by its ID.
func (s *SecretService) GetByID(ctx context.Context, id string) (*model.Secret, error) {
	secret, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get secret by id: %w", err)
	}
	return secret, nil
}

// GetByName retrieves a secret by its name.
func (s *SecretService) GetByName(ctx context.Context, name string) (*model.Secret, error) {
	secret, err := s.repo.GetByName(ctx, name)
	if err != nil {
		return nil, fmt.Errorf("get secret by name: %w", err)
	}
	return secret, nil
}

// List retrieves a list of secrets with pagination.
func (s *SecretService) List(ctx context.Context, limit, offset int) ([]*model.Secret, error) {
	secrets, err := s.repo.List(ctx, limit, offset)
	if err != nil {
		return nil, fmt.Errorf("list secrets: %w", err)
	}
	return secrets, nil
}

// Update updates an existing secret with the given request parameters.
// If refresh configuration is changed, it will validate the script and reconcile the scheduled refresh job.
func (s *SecretService) Update(
	ctx context.Context,
	id string,
	req model.UpdateSecretRequest,
) (*model.Secret, error) {
	// Get current secret to check if validation is needed
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("get current secret: %w", err)
	}

	// Validate script if needed
	if s.shouldValidateRefreshOnUpdate(current, req) {
		if validateErr := s.validateRefreshUpdate(ctx, id, req); validateErr != nil {
			return nil, validateErr
		}
	}

	secret, err := s.repo.Update(ctx, id, req)
	if err != nil {
		return nil, fmt.Errorf("update secret: %w", err)
	}

	// Reconcile refresh schedule if refresh configuration was updated
	s.reconcileRefreshScheduleAfterUpdate(ctx, secret, req)

	// Optional: Log success (avoid logging sensitive names)
	if s.logger != nil && secret != nil {
		s.logger.DebugContext(ctx, "secret updated", "id", secret.ID, "refresh_enabled", secret.RefreshEnabled)
	}

	return secret, nil
}

// shouldValidateRefreshOnUpdate determines if refresh validation is needed for an update.
func (s *SecretService) shouldValidateRefreshOnUpdate(current *model.Secret, req model.UpdateSecretRequest) bool {
	// Validate if enabling refresh
	if req.RefreshEnabled != nil && *req.RefreshEnabled {
		return true
	}

	// Validate if already has refresh enabled AND (script path or env config is being changed)
	if current.RefreshEnabled && (req.ProviderScriptPath != nil || req.EnvConfig != nil) {
		return true
	}

	return false
}

// reconcileRefreshScheduleAfterUpdate reconciles the refresh schedule after an update.
func (s *SecretService) reconcileRefreshScheduleAfterUpdate(
	ctx context.Context,
	secret *model.Secret,
	req model.UpdateSecretRequest,
) {
	if s.refreshSvc == nil {
		return
	}
	refreshConfigChanged := req.RefreshEnabled != nil || req.ProviderScriptPath != nil || req.RefreshInterval != nil
	if !refreshConfigChanged {
		return
	}
	if err := s.refreshSvc.ReconcileSchedule(ctx, secret); err != nil {
		// Log error but don't fail the update operation
		if s.logger != nil {
			s.logger.ErrorContext(ctx, "failed to reconcile secret refresh schedule",
				"secret_id", secret.ID,
				"error", err)
		}
	}
}

// validateRefreshUpdate validates that enabling refresh will have required fields and that the script works.
func (s *SecretService) validateRefreshUpdate(ctx context.Context, id string, req model.UpdateSecretRequest) error {
	// Fetch current secret to check existing values
	current, err := s.repo.GetByID(ctx, id)
	if err != nil {
		return fmt.Errorf("get current secret: %w", err)
	}

	// Determine final values after update
	finalProviderPath, finalInterval, finalEnvConfig := s.computeFinalRefreshValues(current, req)

	// Validate required fields
	if validateErr := s.validateRefreshFields(finalProviderPath, finalInterval); validateErr != nil {
		return validateErr
	}

	// Validate that the provider script executes successfully
	return s.validateProviderScript(ctx, finalProviderPath, finalEnvConfig)
}

// computeFinalRefreshValues computes the final refresh configuration values after an update.
func (s *SecretService) computeFinalRefreshValues(
	current *model.Secret,
	req model.UpdateSecretRequest,
) (*string, *int64, json.RawMessage) {
	finalProviderPath := current.ProviderScriptPath
	if req.ProviderScriptPath != nil {
		finalProviderPath = req.ProviderScriptPath
	}

	finalInterval := current.RefreshInterval
	if req.RefreshInterval != nil {
		finalInterval = req.RefreshInterval
	}

	finalEnvConfig := current.EnvConfig
	if req.EnvConfig != nil {
		finalEnvConfig = s.parseEnvConfig(req.EnvConfig)
	}

	return finalProviderPath, finalInterval, finalEnvConfig
}

// validateRefreshFields validates that required refresh fields are present.
func (s *SecretService) validateRefreshFields(providerPath *string, interval *int64) error {
	if providerPath == nil || *providerPath == "" {
		return errors.New("provider_script_path is required when enabling refresh")
	}
	if interval == nil || *interval <= 0 {
		return errors.New("refresh_interval_seconds is required when enabling refresh")
	}
	return nil
}

// validateProviderScript validates that the provider script executes successfully.
func (s *SecretService) validateProviderScript(
	ctx context.Context,
	providerPath *string,
	envConfig json.RawMessage,
) error {
	if s.refreshSvc == nil {
		return errors.New("refresh service not available")
	}

	tempSecret := &model.Secret{
		ProviderScriptPath: providerPath,
		EnvConfig:          envConfig,
		RefreshEnabled:     true,
	}

	if _, err := s.refreshSvc.ExecuteProviderScript(ctx, tempSecret); err != nil {
		return s.providerScriptError(err, providerPath)
	}

	return nil
}

// providerScriptError wraps provider script failures with a user-facing error and logs details.
func (s *SecretService) providerScriptError(err error, providerPath *string) error {
	if err == nil {
		return nil
	}

	scriptPath := ""
	if providerPath != nil {
		scriptPath = strings.TrimSpace(*providerPath)
	}

	logger := s.logger
	if logger == nil {
		logger = slog.Default()
	}
	logger.Error("provider script validation failed",
		"script_path", scriptPath,
		"error", err)

	return NewSecretProviderScriptError(scriptPath, err)
}

// Delete deletes a secret by its ID.
// If the secret has refresh enabled, it will also remove the scheduled refresh job.
func (s *SecretService) Delete(ctx context.Context, id string) (bool, error) {
	// Get secret first to check if it has refresh enabled
	secret, err := s.repo.GetByID(ctx, id)
	if err == nil && secret.RefreshEnabled && s.refreshSvc != nil {
		// Remove scheduled job (best effort - don't fail delete if this fails)
		if removeErr := s.refreshSvc.RemoveSchedule(ctx, id); removeErr != nil && s.logger != nil {
			s.logger.WarnContext(
				ctx,
				"failed to remove refresh schedule during secret delete",
				"id",
				id,
				"error",
				removeErr,
			)
		}
	}

	deleted, err := s.repo.Delete(ctx, id)
	if err != nil {
		return false, fmt.Errorf("delete secret: %w", err)
	}

	// Optional: Log success
	if s.logger != nil && deleted {
		s.logger.DebugContext(ctx, "secret deleted", "id", id)
	}

	return deleted, nil
}
