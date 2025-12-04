//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

const (
	maxSiteNameLen = 255
)

// SiteAlertMode controls how alerts are delivered for a site.
type SiteAlertMode string

const (
	SiteAlertModeActive SiteAlertMode = "active"
	SiteAlertModeMuted  SiteAlertMode = "muted"
)

// Valid reports whether the site alert mode is supported.
func (m SiteAlertMode) Valid() bool {
	switch m {
	case SiteAlertModeActive, SiteAlertModeMuted:
		return true
	default:
		return false
	}
}

// normalizeSiteAlertMode trims and lowercases the input, defaulting to active when empty.
func normalizeSiteAlertMode(v SiteAlertMode) SiteAlertMode {
	normalized := SiteAlertMode(strings.ToLower(strings.TrimSpace(string(v))))
	if normalized == "" {
		return SiteAlertModeActive
	}
	return normalized
}

// ParseSiteAlertMode normalizes an alert mode string and reports whether it is supported.
func ParseSiteAlertMode(value string) (SiteAlertMode, bool) {
	mode := SiteAlertMode(strings.ToLower(strings.TrimSpace(value)))
	if mode.Valid() {
		return mode, true
	}
	return "", false
}

// SitesListOptions controls paging and filtering for listing sites.
// Notes:
// - Sort supports: "created_at", "name" (case-insensitive).
// - Dir supports: "asc", "desc" (case-insensitive); values are normalized internally.
// - Q matches name via ILIKE substring.
// - Scope and Enabled match exactly.
type SitesListOptions struct {
	Limit   int
	Offset  int
	Q       *string // substring match on name (ILIKE)
	Enabled *bool   // exact match
	Scope   *string // exact match
	Sort    string  // allowed: "created_at", "name"
	Dir     string  // allowed: "asc", "desc" (case-insensitive; normalized internally)
}

// Site represents a monitored site configuration.
type Site struct {
	ID              string        `json:"id"                           db:"id"`
	Name            string        `json:"name"                         db:"name"`
	Enabled         bool          `json:"enabled"                      db:"enabled"`
	AlertMode       SiteAlertMode `json:"alert_mode"                   db:"alert_mode"`
	Scope           *string       `json:"scope,omitempty"              db:"scope"`
	HTTPAlertSinkID *string       `json:"http_alert_sink_id,omitempty" db:"http_alert_sink_id"`
	LastEnabled     *time.Time    `json:"last_enabled,omitempty"       db:"last_enabled"`
	LastRun         *time.Time    `json:"last_run,omitempty"           db:"last_run"`
	RunEveryMinutes int           `json:"run_every_minutes"            db:"run_every_minutes"`
	SourceID        string        `json:"source_id"                    db:"source_id"`
	CreatedAt       time.Time     `json:"created_at"                   db:"created_at"`
	UpdatedAt       time.Time     `json:"updated_at"                   db:"updated_at"`
}

// CreateSiteRequest represents parameters to create a Site.
type CreateSiteRequest struct {
	Name            string        `json:"name"`
	Enabled         *bool         `json:"enabled,omitempty"`
	AlertMode       SiteAlertMode `json:"alert_mode,omitempty"`
	Scope           *string       `json:"scope,omitempty"`
	HTTPAlertSinkID *string       `json:"http_alert_sink_id,omitempty"`
	RunEveryMinutes int           `json:"run_every_minutes"`
	SourceID        string        `json:"source_id"`
}

// UpdateSiteRequest represents parameters to update a Site.
type UpdateSiteRequest struct {
	Name            *string        `json:"name,omitempty"`
	Enabled         *bool          `json:"enabled,omitempty"`
	AlertMode       *SiteAlertMode `json:"alert_mode,omitempty"`
	Scope           *string        `json:"scope,omitempty"`
	HTTPAlertSinkID *string        `json:"http_alert_sink_id,omitempty"`
	RunEveryMinutes *int           `json:"run_every_minutes,omitempty"`
	SourceID        *string        `json:"source_id,omitempty"`
}

// Validate validates CreateSiteRequest.
func (r *CreateSiteRequest) Validate() error {
	name := strings.TrimSpace(r.Name)
	if name == "" {
		return errors.New("name is required and cannot be empty")
	}
	if utf8.RuneCountInString(name) > maxSiteNameLen {
		return errors.New("name cannot exceed 255 characters")
	}
	if r.RunEveryMinutes <= 0 {
		return errors.New("run_every_minutes must be > 0")
	}
	if strings.TrimSpace(r.SourceID) == "" {
		return errors.New("source_id is required")
	}
	r.AlertMode = normalizeSiteAlertMode(r.AlertMode)
	if !r.AlertMode.Valid() {
		return errors.New("invalid alert_mode")
	}
	return nil
}

// HasUpdates reports whether any field is set in UpdateSiteRequest.
func (r *UpdateSiteRequest) HasUpdates() bool {
	return r.Name != nil || r.Enabled != nil || r.AlertMode != nil || r.Scope != nil || r.HTTPAlertSinkID != nil ||
		r.RunEveryMinutes != nil ||
		r.SourceID != nil
}

// Validate validates UpdateSiteRequest, ensuring at least one field is set and values are sane.
func (r *UpdateSiteRequest) Validate() error {
	if !r.HasUpdates() {
		return errors.New("at least one field must be updated")
	}
	if r.Name != nil {
		n := strings.TrimSpace(*r.Name)
		if n == "" {
			return errors.New("name cannot be empty")
		}
		if utf8.RuneCountInString(n) > maxSiteNameLen {
			return errors.New("name cannot exceed 255 characters")
		}
	}
	if r.RunEveryMinutes != nil && *r.RunEveryMinutes <= 0 {
		return errors.New("run_every_minutes must be > 0")
	}
	if r.SourceID != nil && strings.TrimSpace(*r.SourceID) == "" {
		return errors.New("source_id cannot be empty")
	}
	if r.AlertMode != nil {
		mode := normalizeSiteAlertMode(*r.AlertMode)
		if !mode.Valid() {
			return errors.New("invalid alert_mode")
		}
		*r.AlertMode = mode
	}
	return nil
}
