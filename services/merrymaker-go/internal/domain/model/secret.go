//revive:disable-next-line:var-naming // legacy package name used across the project
package model

import (
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxSecretNameLen = 255
)

var secretNameRe = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]*$`)

func validateSecretNameRequired(name string) error {
	n := strings.TrimSpace(name)
	if n == "" {
		return errors.New("name is required and cannot be empty")
	}
	if utf8.RuneCountInString(n) > maxSecretNameLen {
		return errors.New("name cannot exceed 255 characters")
	}
	if !secretNameRe.MatchString(n) {
		return errors.New(
			"name must start with a letter, digit, or underscore and contain only letters, digits, underscores, or hyphens",
		)
	}
	return nil
}

func validateSecretNameProvided(name string) error {
	n := strings.TrimSpace(name)
	if n == "" {
		return errors.New("name cannot be empty")
	}
	if utf8.RuneCountInString(n) > maxSecretNameLen {
		return errors.New("name cannot exceed 255 characters")
	}
	if !secretNameRe.MatchString(n) {
		return errors.New(
			"name must start with a letter, digit, or underscore and contain only letters, digits, underscores, or hyphens",
		)
	}
	return nil
}

// Secret represents a stored secret. Value is decrypted when fetched via repo Get* methods.
// Secrets can be static (manually set) or dynamic (automatically refreshed via provider scripts).
type Secret struct {
	ID        string    `json:"id"              db:"id"`
	Name      string    `json:"name"            db:"name"`
	Value     string    `json:"value,omitempty" db:"value"`
	CreatedAt time.Time `json:"created_at"      db:"created_at"`
	UpdatedAt time.Time `json:"updated_at"      db:"updated_at"`

	// Refresh configuration (optional - for dynamic secrets)
	ProviderScriptPath *string         `json:"provider_script_path,omitempty"     db:"provider_script_path"`
	EnvConfig          json.RawMessage `json:"env_config,omitempty"               db:"env_config"`               // JSONB stored as []byte
	RefreshInterval    *int64          `json:"refresh_interval_seconds,omitempty" db:"refresh_interval_seconds"` // Parsed from INTERVAL in seconds
	LastRefreshedAt    *time.Time      `json:"last_refreshed_at,omitempty"        db:"last_refreshed_at"`
	LastRefreshStatus  *string         `json:"last_refresh_status,omitempty"      db:"last_refresh_status"`
	LastRefreshError   *string         `json:"last_refresh_error,omitempty"       db:"last_refresh_error"`
	RefreshEnabled     bool            `json:"refresh_enabled"                    db:"refresh_enabled"`
}

// CreateSecretRequest contains fields to create a new secret.
// For static secrets: provide Name and Value.
// For dynamic secrets: provide Name, ProviderScriptPath, RefreshInterval, and optionally EnvConfig.
// Value is optional for dynamic secrets (will be populated on first refresh).
type CreateSecretRequest struct {
	Name  string `json:"name"`
	Value string `json:"value"` // Required for static secrets, optional for dynamic

	// Refresh configuration (optional - for dynamic secrets)
	ProviderScriptPath *string `json:"provider_script_path,omitempty"`
	EnvConfig          *string `json:"env_config,omitempty"` // JSON string
	RefreshInterval    *int64  `json:"refresh_interval_seconds,omitempty"`
	RefreshEnabled     *bool   `json:"refresh_enabled,omitempty"`
}

func (r *CreateSecretRequest) Validate() error {
	if err := validateSecretNameRequired(r.Name); err != nil {
		return err
	}

	// Determine if this is a dynamic (refreshable) secret
	refreshEnabled := r.RefreshEnabled != nil && *r.RefreshEnabled

	if refreshEnabled {
		return r.validateDynamicSecret()
	}
	return r.validateStaticSecret()
}

// validateDynamicSecret validates a dynamic (refreshable) secret.
func (r *CreateSecretRequest) validateDynamicSecret() error {
	if r.ProviderScriptPath == nil || strings.TrimSpace(*r.ProviderScriptPath) == "" {
		return errors.New("provider_script_path is required when refresh_enabled is true")
	}
	if r.RefreshInterval == nil || *r.RefreshInterval <= 0 {
		return errors.New("refresh_interval_seconds must be positive when refresh_enabled is true")
	}
	// Value is optional for dynamic secrets (will be populated on first refresh)
	// but if provided, it must not be empty
	if r.Value != "" && strings.TrimSpace(r.Value) == "" {
		return errors.New("value cannot be whitespace-only")
	}
	return nil
}

// validateStaticSecret validates a static secret.
func (r *CreateSecretRequest) validateStaticSecret() error {
	if strings.TrimSpace(r.Value) == "" {
		return errors.New("value is required for static secrets")
	}
	return nil
}

// UpdateSecretRequest supports updating name/value and refresh configuration.
type UpdateSecretRequest struct {
	Name  *string `json:"name,omitempty"`
	Value *string `json:"value,omitempty"`

	// Refresh configuration updates
	ProviderScriptPath *string `json:"provider_script_path,omitempty"`
	EnvConfig          *string `json:"env_config,omitempty"` // JSON string
	RefreshInterval    *int64  `json:"refresh_interval_seconds,omitempty"`
	RefreshEnabled     *bool   `json:"refresh_enabled,omitempty"`
}

func (r *UpdateSecretRequest) HasUpdates() bool {
	return r.Name != nil || r.Value != nil ||
		r.ProviderScriptPath != nil || r.EnvConfig != nil ||
		r.RefreshInterval != nil || r.RefreshEnabled != nil
}

func (r *UpdateSecretRequest) Validate() error {
	if !r.HasUpdates() {
		return errors.New("at least one field must be updated")
	}
	if err := r.validateBasicFields(); err != nil {
		return err
	}
	return r.validateRefreshFields()
}

// validateBasicFields validates name and value fields.
func (r *UpdateSecretRequest) validateBasicFields() error {
	if r.Name != nil {
		if err := validateSecretNameProvided(*r.Name); err != nil {
			return err
		}
	}
	if r.Value != nil && strings.TrimSpace(*r.Value) == "" {
		return errors.New("value cannot be empty")
	}
	return nil
}

// validateRefreshFields validates refresh configuration fields.
func (r *UpdateSecretRequest) validateRefreshFields() error {
	if r.RefreshEnabled == nil || !*r.RefreshEnabled {
		return nil
	}
	// If enabling refresh, ensure required fields are valid if provided
	if r.ProviderScriptPath != nil && strings.TrimSpace(*r.ProviderScriptPath) == "" {
		return errors.New("provider_script_path cannot be empty when refresh is enabled")
	}
	if r.RefreshInterval != nil && *r.RefreshInterval <= 0 {
		return errors.New("refresh_interval_seconds must be positive when refresh is enabled")
	}
	return nil
}
