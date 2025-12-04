//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
)

// Rule represents a detection rule in the system.
type Rule struct {
	ID        string          `json:"id"         db:"id"`
	SiteID    string          `json:"site_id"    db:"site_id"`
	RuleType  string          `json:"rule_type"  db:"rule_type"`
	Config    json.RawMessage `json:"config"     db:"config"`
	Enabled   bool            `json:"enabled"    db:"enabled"`
	Priority  int             `json:"priority"   db:"priority"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}

// RuleType represents the type of detection rule.
type RuleType string

const (
	RuleTypeUnknownDomain RuleType = "unknown_domain"
	RuleTypeIOC           RuleType = "ioc_domain"
	RuleTypeYaraRule      RuleType = "yara_rule"
	RuleTypeCustom        RuleType = "custom"
)

// Valid returns true if the rule type is valid.
func (t RuleType) Valid() bool {
	switch t {
	case RuleTypeUnknownDomain, RuleTypeIOC, RuleTypeYaraRule, RuleTypeCustom:
		return true
	default:
		return false
	}
}

// String returns the string representation of the rule type.
func (t RuleType) String() string {
	return string(t)
}

// CreateRuleRequest represents a request to create a new rule.
type CreateRuleRequest struct {
	SiteID   string          `json:"site_id"`
	RuleType string          `json:"rule_type"`
	Config   json.RawMessage `json:"config,omitempty"`
	Enabled  *bool           `json:"enabled,omitempty"`
	Priority *int            `json:"priority,omitempty"`
}

// Normalize normalizes the CreateRuleRequest fields.
func (r *CreateRuleRequest) Normalize() {
	r.SiteID = strings.TrimSpace(r.SiteID)
	r.RuleType = strings.TrimSpace(r.RuleType)
}

// Validate validates the CreateRuleRequest fields.
func (r *CreateRuleRequest) Validate() error {
	if r.SiteID == "" {
		return errors.New("site_id is required")
	}

	if !RuleType(r.RuleType).Valid() {
		return errors.New("invalid rule_type")
	}

	if r.Priority != nil && (*r.Priority < 1 || *r.Priority > 1000) {
		return errors.New("priority must be between 1 and 1000")
	}

	return nil
}

// UpdateRuleRequest represents a request to update an existing rule.
type UpdateRuleRequest struct {
	RuleType *string         `json:"rule_type,omitempty"`
	Config   json.RawMessage `json:"config,omitempty"`
	Enabled  *bool           `json:"enabled,omitempty"`
	Priority *int            `json:"priority,omitempty"`
}

// HasUpdates reports whether any field is set in UpdateRuleRequest.
func (r *UpdateRuleRequest) HasUpdates() bool {
	return r.RuleType != nil || r.Config != nil || r.Enabled != nil || r.Priority != nil
}

// Validate validates UpdateRuleRequest, ensuring at least one field is set and values are sane.
func (r *UpdateRuleRequest) Validate() error {
	if !r.HasUpdates() {
		return errors.New("at least one field must be updated")
	}

	if r.RuleType != nil {
		if !RuleType(*r.RuleType).Valid() {
			return errors.New("invalid rule_type")
		}
	}

	if r.Priority != nil && (*r.Priority < 1 || *r.Priority > 1000) {
		return errors.New("priority must be between 1 and 1000")
	}

	return nil
}

// RuleListOptions represents options for listing rules.
type RuleListOptions struct {
	SiteID   *string `json:"site_id,omitempty"`
	RuleType *string `json:"rule_type,omitempty"`
	Enabled  *bool   `json:"enabled,omitempty"`
	Limit    int     `json:"limit,omitempty"`
	Offset   int     `json:"offset,omitempty"`
}

// UnknownDomainRuleConfig represents configuration for unknown domain detection rules.
type UnknownDomainRuleConfig struct {
	AllowedPatterns []string `json:"allowed_patterns,omitempty"` // Glob patterns for allowed domains
	Scope           string   `json:"scope,omitempty"`            // Scope context for domain tracking
	Cooldown        int      `json:"cooldown,omitempty"`         // Cooldown period in minutes before re-alerting
}

// IOCRuleConfig represents configuration for IOC detection rules.
type IOCRuleConfig struct {
	Scope          string   `json:"scope,omitempty"`           // Scope context for IOC checking
	MinSeverity    string   `json:"min_severity,omitempty"`    // Minimum IOC severity to alert on
	ExcludeSources []string `json:"exclude_sources,omitempty"` // IOC sources to exclude
	Cooldown       int      `json:"cooldown,omitempty"`        // Cooldown period in minutes before re-alerting
}

// YaraRuleConfig represents configuration for YARA rule processing.
type YaraRuleConfig struct {
	Scope       string   `json:"scope,omitempty"`         // Scope context for file processing
	RuleFiles   []string `json:"rule_files,omitempty"`    // YARA rule file paths
	FileTypes   []string `json:"file_types,omitempty"`    // File types to scan (e.g., "application/pdf")
	MaxFileSize int64    `json:"max_file_size,omitempty"` // Maximum file size to scan in bytes
}

// CustomRuleConfig represents configuration for custom rules.
type CustomRuleConfig struct {
	Script     string                 `json:"script,omitempty"`     // Custom rule script/logic
	Parameters map[string]interface{} `json:"parameters,omitempty"` // Custom rule parameters
	Scope      string                 `json:"scope,omitempty"`      // Scope context for custom rule
}
