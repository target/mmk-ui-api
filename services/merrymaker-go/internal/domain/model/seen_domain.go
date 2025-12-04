//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"errors"
	"strings"
	"time"
)

// SeenDomain represents a domain that has been observed for a specific site and scope.
type SeenDomain struct {
	ID          string    `json:"id"            db:"id"`
	SiteID      string    `json:"site_id"       db:"site_id"`
	Domain      string    `json:"domain"        db:"domain"`
	Scope       string    `json:"scope"         db:"scope"`
	FirstSeenAt time.Time `json:"first_seen_at" db:"first_seen_at"`
	LastSeenAt  time.Time `json:"last_seen_at"  db:"last_seen_at"`
	HitCount    int       `json:"hit_count"     db:"hit_count"`
	CreatedAt   time.Time `json:"created_at"    db:"created_at"`
}

// CreateSeenDomainRequest represents a request to create a new seen domain record.
type CreateSeenDomainRequest struct {
	SiteID      string     `json:"site_id"`
	Domain      string     `json:"domain"`
	Scope       string     `json:"scope,omitempty"`
	FirstSeenAt *time.Time `json:"first_seen_at,omitempty"`
	LastSeenAt  *time.Time `json:"last_seen_at,omitempty"`
}

// Normalize normalizes the CreateSeenDomainRequest fields.
func (r *CreateSeenDomainRequest) Normalize() {
	r.SiteID = strings.TrimSpace(r.SiteID)
	r.Domain = strings.TrimSpace(strings.ToLower(r.Domain))
	r.Scope = strings.TrimSpace(r.Scope)
	if r.Scope == "" {
		r.Scope = DefaultScope
	}
}

// Validate validates the CreateSeenDomainRequest fields.
func (r *CreateSeenDomainRequest) Validate() error {
	if r.SiteID == "" {
		return errors.New("site_id is required")
	}

	if r.Domain == "" {
		return errors.New("domain is required")
	}

	// Basic domain validation - should contain at least one dot and valid characters
	if !strings.Contains(r.Domain, ".") {
		return errors.New("domain must be a valid domain name")
	}

	return nil
}

// UpdateSeenDomainRequest represents a request to update an existing seen domain record.
type UpdateSeenDomainRequest struct {
	LastSeenAt *time.Time `json:"last_seen_at,omitempty"`
	HitCount   *int       `json:"hit_count,omitempty"`
}

// HasUpdates reports whether any field is set in UpdateSeenDomainRequest.
func (r *UpdateSeenDomainRequest) HasUpdates() bool {
	return r.LastSeenAt != nil || r.HitCount != nil
}

// Validate validates UpdateSeenDomainRequest, ensuring at least one field is set and values are sane.
func (r *UpdateSeenDomainRequest) Validate() error {
	if !r.HasUpdates() {
		return errors.New("at least one field must be updated")
	}

	if r.HitCount != nil && *r.HitCount < 1 {
		return errors.New("hit_count must be >= 1")
	}

	return nil
}

// SeenDomainListOptions represents options for listing seen domains.
type SeenDomainListOptions struct {
	SiteID *string `json:"site_id,omitempty"`
	Scope  *string `json:"scope,omitempty"`
	Domain *string `json:"domain,omitempty"` // Partial domain match
	Limit  int     `json:"limit,omitempty"`
	Offset int     `json:"offset,omitempty"`
}

// SeenDomainLookupRequest represents a request to check if a domain has been seen.
type SeenDomainLookupRequest struct {
	SiteID string `json:"site_id"`
	Domain string `json:"domain"`
	Scope  string `json:"scope,omitempty"`
}

// Normalize normalizes the SeenDomainLookupRequest fields.
func (r *SeenDomainLookupRequest) Normalize() {
	r.SiteID = strings.TrimSpace(r.SiteID)
	r.Domain = strings.TrimSpace(strings.ToLower(r.Domain))
	r.Scope = strings.TrimSpace(r.Scope)
	if r.Scope == "" {
		r.Scope = DefaultScope
	}
}

// Validate validates the SeenDomainLookupRequest fields.
func (r *SeenDomainLookupRequest) Validate() error {
	if r.SiteID == "" {
		return errors.New("site_id is required")
	}

	if r.Domain == "" {
		return errors.New("domain is required")
	}

	return nil
}

// RecordDomainSeenRequest represents a request to record a domain as seen (upsert operation).
type RecordDomainSeenRequest struct {
	SiteID string     `json:"site_id"`
	Domain string     `json:"domain"`
	Scope  string     `json:"scope,omitempty"`
	SeenAt *time.Time `json:"seen_at,omitempty"`
}

// Normalize normalizes the RecordDomainSeenRequest fields.
func (r *RecordDomainSeenRequest) Normalize() {
	r.SiteID = strings.TrimSpace(r.SiteID)
	r.Domain = strings.TrimSpace(strings.ToLower(r.Domain))
	r.Scope = strings.TrimSpace(r.Scope)
	if r.Scope == "" {
		r.Scope = DefaultScope
	}
}

// Validate validates the RecordDomainSeenRequest fields.
func (r *RecordDomainSeenRequest) Validate() error {
	if r.SiteID == "" {
		return errors.New("site_id is required")
	}

	if r.Domain == "" {
		return errors.New("domain is required")
	}

	return nil
}
