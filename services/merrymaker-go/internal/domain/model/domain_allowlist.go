//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
)

// DomainAllowlist represents a domain allowlist entry with pattern matching support.
type DomainAllowlist struct {
	ID          string    `json:"id"                    db:"id"`
	Scope       string    `json:"scope"                 db:"scope"`        // Scope context; 'global' for global allowlists
	Pattern     string    `json:"pattern"               db:"pattern"`      // Domain pattern
	PatternType string    `json:"pattern_type"          db:"pattern_type"` // exact, wildcard, glob, etld_plus_one
	Description string    `json:"description,omitempty" db:"description"`
	Enabled     bool      `json:"enabled"               db:"enabled"`
	Priority    int       `json:"priority"              db:"priority"`
	CreatedAt   time.Time `json:"created_at"            db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"            db:"updated_at"`
}

// IsGlobal returns true if this is a global allowlist entry (scope is 'global').
func (d *DomainAllowlist) IsGlobal() bool {
	return d.Scope == ScopeGlobal
}

// PatternType constants for domain allowlist patterns.
const (
	PatternTypeExact       = "exact"         // Exact domain match
	PatternTypeWildcard    = "wildcard"      // Simple wildcard matching (*.example.com)
	PatternTypeGlob        = "glob"          // Full glob pattern matching
	PatternTypeETLDPlusOne = "etld_plus_one" // eTLD+1 matching (example.com matches sub.example.com)
)

// ScopeGlobal identifies the global scope for domain allowlists.
const ScopeGlobal = "global"

// ValidPatternTypes returns all valid pattern types.
func ValidPatternTypes() []string {
	return []string{PatternTypeExact, PatternTypeWildcard, PatternTypeGlob, PatternTypeETLDPlusOne}
}

// IsValidPatternType checks if a pattern type is valid.
func IsValidPatternType(patternType string) bool {
	return slices.Contains(ValidPatternTypes(), patternType)
}

// CreateDomainAllowlistRequest represents a request to create a new domain allowlist entry.
type CreateDomainAllowlistRequest struct {
	Scope       string `json:"scope,omitempty"`        // Scope context; defaults to 'default'
	Pattern     string `json:"pattern"`                // Required domain pattern
	PatternType string `json:"pattern_type,omitempty"` // Defaults to 'exact'
	Description string `json:"description,omitempty"`
	Enabled     *bool  `json:"enabled,omitempty"`  // Defaults to true
	Priority    *int   `json:"priority,omitempty"` // Defaults to 100
}

// Normalize normalizes the CreateDomainAllowlistRequest fields.
func (r *CreateDomainAllowlistRequest) Normalize() {
	r.Pattern = strings.TrimSpace(strings.ToLower(r.Pattern))
	r.PatternType = strings.TrimSpace(strings.ToLower(r.PatternType))
	r.Scope = strings.TrimSpace(r.Scope)
	r.Description = strings.TrimSpace(r.Description)

	// Set defaults
	if r.PatternType == "" {
		r.PatternType = PatternTypeExact
	}
	if r.Scope == "" {
		r.Scope = "default"
	}
	if r.Enabled == nil {
		enabled := true
		r.Enabled = &enabled
	}
	if r.Priority == nil {
		priority := 100
		r.Priority = &priority
	}
}

// Validate validates the CreateDomainAllowlistRequest fields.
//

func (r *CreateDomainAllowlistRequest) Validate() error {
	if r.Pattern == "" {
		return errors.New("pattern is required and cannot be empty")
	}

	if !utf8.ValidString(r.Pattern) {
		return errors.New("pattern must be valid UTF-8")
	}

	if len(r.Pattern) > 255 {
		return errors.New("pattern cannot exceed 255 characters")
	}

	if !IsValidPatternType(r.PatternType) {
		return fmt.Errorf("pattern_type must be one of: %s", strings.Join(ValidPatternTypes(), ", "))
	}

	if r.Scope == "" {
		return errors.New("scope is required and cannot be empty")
	}

	if len(r.Scope) > 100 {
		return errors.New("scope cannot exceed 100 characters")
	}

	if len(r.Description) > 1000 {
		return errors.New("description cannot exceed 1000 characters")
	}

	if r.Priority != nil && (*r.Priority < 1 || *r.Priority > 1000) {
		return errors.New("priority must be between 1 and 1000")
	}

	// No additional scope validation needed - any scope is valid

	return nil
}

// UpdateDomainAllowlistRequest represents a request to update an existing domain allowlist entry.
type UpdateDomainAllowlistRequest struct {
	Scope       *string `json:"scope,omitempty"`        // Scope context
	Pattern     *string `json:"pattern,omitempty"`      // Domain pattern
	PatternType *string `json:"pattern_type,omitempty"` // Pattern type
	Description *string `json:"description,omitempty"`  // Description
	Enabled     *bool   `json:"enabled,omitempty"`      // Enabled status
	Priority    *int    `json:"priority,omitempty"`     // Priority
}

// Normalize normalizes the UpdateDomainAllowlistRequest fields.
func (r *UpdateDomainAllowlistRequest) Normalize() {
	if r.Scope != nil {
		normalized := strings.TrimSpace(*r.Scope)
		r.Scope = &normalized
	}
	if r.Pattern != nil {
		normalized := strings.TrimSpace(strings.ToLower(*r.Pattern))
		r.Pattern = &normalized
	}
	if r.PatternType != nil {
		normalized := strings.TrimSpace(strings.ToLower(*r.PatternType))
		r.PatternType = &normalized
	}
	if r.Description != nil {
		normalized := strings.TrimSpace(*r.Description)
		r.Description = &normalized
	}
}

// Validate validates the UpdateDomainAllowlistRequest fields.
//

func (r *UpdateDomainAllowlistRequest) Validate() error {
	hasUpdate := r.Scope != nil || r.Pattern != nil || r.PatternType != nil || r.Description != nil ||
		r.Enabled != nil ||
		r.Priority != nil
	if !hasUpdate {
		return errors.New("at least one field must be updated")
	}

	if err := validateOptionalScope(r.Scope); err != nil {
		return err
	}

	if err := validateOptionalPattern(r.Pattern); err != nil {
		return err
	}

	if r.PatternType != nil && !IsValidPatternType(*r.PatternType) {
		return fmt.Errorf("pattern_type must be one of: %s", strings.Join(ValidPatternTypes(), ", "))
	}

	if r.Description != nil && len(*r.Description) > 1000 {
		return errors.New("description cannot exceed 1000 characters")
	}

	if r.Priority != nil && (*r.Priority < 1 || *r.Priority > 1000) {
		return errors.New("priority must be between 1 and 1000")
	}

	return nil
}

func validateOptionalScope(scope *string) error {
	if scope == nil {
		return nil
	}

	if len(*scope) > 100 {
		return errors.New("scope cannot exceed 100 characters")
	}

	return nil
}

func validateOptionalPattern(pattern *string) error {
	if pattern == nil {
		return nil
	}

	if *pattern == "" {
		return errors.New("pattern cannot be empty")
	}
	if !utf8.ValidString(*pattern) {
		return errors.New("pattern must be valid UTF-8")
	}
	if len(*pattern) > 255 {
		return errors.New("pattern cannot exceed 255 characters")
	}

	return nil
}

// DomainAllowlistListOptions represents options for listing domain allowlist entries.
type DomainAllowlistListOptions struct {
	Scope       *string `json:"scope,omitempty"`        // Filter by scope
	Pattern     *string `json:"pattern,omitempty"`      // Partial pattern match
	PatternType *string `json:"pattern_type,omitempty"` // Filter by pattern type
	Enabled     *bool   `json:"enabled,omitempty"`      // Filter by enabled status
	GlobalOnly  *bool   `json:"global_only,omitempty"`  // Show only global entries
	Limit       int     `json:"limit,omitempty"`
	Offset      int     `json:"offset,omitempty"`
}

// DomainAllowlistLookupRequest represents a request to check if a domain is allowed or to fetch patterns for a scope.
type DomainAllowlistLookupRequest struct {
	Scope  string `json:"scope"`  // Required scope context
	Domain string `json:"domain"` // Domain to check (optional when listing patterns)
}

// Normalize normalizes the DomainAllowlistLookupRequest fields.
func (r *DomainAllowlistLookupRequest) Normalize() {
	r.Domain = strings.TrimSpace(strings.ToLower(r.Domain))
	r.Scope = strings.TrimSpace(r.Scope)
}

// Validate validates the DomainAllowlistLookupRequest fields.
// Note: Domain may be empty when used to fetch patterns for a scope (GetForScope).
func (r *DomainAllowlistLookupRequest) Validate() error {
	if r.Scope == "" {
		return errors.New("scope is required")
	}
	return nil
}

// DomainAllowlistStats represents statistics about domain allowlist entries.
type DomainAllowlistStats struct {
	Total         int `json:"total"`
	Global        int `json:"global"`         // Global entries
	Scoped        int `json:"scoped"`         // Site-scoped entries
	Enabled       int `json:"enabled"`        // Enabled entries
	Disabled      int `json:"disabled"`       // Disabled entries
	ExactCount    int `json:"exact_count"`    // Exact pattern entries
	WildcardCount int `json:"wildcard_count"` // Wildcard pattern entries
	GlobCount     int `json:"glob_count"`     // Glob pattern entries
	ETLDCount     int `json:"etld_count"`     // eTLD+1 pattern entries
}
