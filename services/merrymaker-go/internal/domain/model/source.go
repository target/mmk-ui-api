// Package model defines the core data types and structures used throughout the merrymaker job system.
package model

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	// maxNameLen is the maximum allowed length for source names in characters.
	maxNameLen = 255
)

// Source represents a Puppeteer script source in the system.
type Source struct {
	ID        string    `json:"id"         db:"id"`
	Name      string    `json:"name"       db:"name"`
	Value     string    `json:"value"      db:"value"`
	Test      bool      `json:"test"       db:"test"`
	Secrets   []string  `json:"secrets"    db:"secrets"`
	CreatedAt time.Time `json:"created_at" db:"created_at"`
}

// CreateSourceRequest represents a request to create a new source.
type CreateSourceRequest struct {
	Name        string   `json:"name"`
	Value       string   `json:"value"`
	Test        bool     `json:"test,omitempty"`
	Secrets     []string `json:"secrets,omitempty"`
	ClientToken string   `json:"client_token,omitempty"`
}

// UpdateSourceRequest represents a request to update an existing source.
type UpdateSourceRequest struct {
	Name    *string  `json:"name,omitempty"`
	Value   *string  `json:"value,omitempty"`
	Test    *bool    `json:"test,omitempty"`
	Secrets []string `json:"secrets,omitempty"`
}

// Validate validates the CreateSourceRequest fields.
func (r *CreateSourceRequest) Validate() error {
	if strings.TrimSpace(r.Name) == "" {
		return errors.New("name is required and cannot be empty")
	}
	if strings.TrimSpace(r.Value) == "" {
		return errors.New("value is required and cannot be empty")
	}
	if utf8.RuneCountInString(r.Name) > maxNameLen {
		return errors.New("name cannot exceed 255 characters")
	}

	return validateSecretsSlice(r.Secrets)
}

// validateSecretsSlice validates that all entries in a secrets slice are non-empty.
func validateSecretsSlice(secrets []string) error {
	for _, secret := range secrets {
		if strings.TrimSpace(secret) == "" {
			return errors.New("secrets cannot contain empty or whitespace-only entries")
		}
	}
	return nil
}

// Validate validates the UpdateSourceRequest fields and ensures at least one field is being updated.
func (r *UpdateSourceRequest) Validate() error {
	if !r.HasUpdates() {
		return errors.New("at least one field must be updated")
	}

	if err := r.validateName(); err != nil {
		return err
	}

	if err := r.validateValue(); err != nil {
		return err
	}

	if err := r.validateSecrets(); err != nil {
		return err
	}

	return nil
}

// validateName validates the name field if it's being updated.
func (r *UpdateSourceRequest) validateName() error {
	if r.Name == nil {
		return nil
	}

	if strings.TrimSpace(*r.Name) == "" {
		return errors.New("name cannot be empty")
	}

	if utf8.RuneCountInString(*r.Name) > maxNameLen {
		return errors.New("name cannot exceed 255 characters")
	}

	return nil
}

// validateValue validates the value field if it's being updated.
func (r *UpdateSourceRequest) validateValue() error {
	if r.Value != nil && strings.TrimSpace(*r.Value) == "" {
		return errors.New("value cannot be empty")
	}
	return nil
}

// validateSecrets validates the secrets field if it's being updated.
func (r *UpdateSourceRequest) validateSecrets() error {
	if r.Secrets == nil {
		return nil
	}

	return validateSecretsSlice(r.Secrets)
}

// HasUpdates returns true if the UpdateSourceRequest has any fields to update.
func (r *UpdateSourceRequest) HasUpdates() bool {
	return r.Name != nil || r.Value != nil || r.Test != nil || r.Secrets != nil
}
