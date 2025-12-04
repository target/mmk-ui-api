//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"errors"
	"fmt"
	"net/netip"
	"regexp"
	"strings"
	"time"
)

// Precompiled domain label regex: labels must start/end alphanumeric; hyphens allowed internally; 1-63 chars.
var domainLabelRe = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

// IOCType represents the type of Indicator of Compromise.
type IOCType string

const (
	IOCTypeFQDN IOCType = "fqdn"
	IOCTypeIP   IOCType = "ip"
)

func (t IOCType) Valid() bool {
	switch t {
	case IOCTypeFQDN, IOCTypeIP:
		return true
	default:
		return false
	}
}

// IOC represents a global IOC record (applies system-wide).
type IOC struct {
	ID          string    `json:"id"                    db:"id"`
	Type        IOCType   `json:"type"                  db:"type"`
	Value       string    `json:"value"                 db:"value"`
	Enabled     bool      `json:"enabled"               db:"enabled"`
	Description *string   `json:"description,omitempty" db:"description"`
	CreatedAt   time.Time `json:"created_at"            db:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"            db:"updated_at"`
}

// CreateIOCRequest holds fields for creating an IOC.
type CreateIOCRequest struct {
	Type        IOCType `json:"type"`
	Value       string  `json:"value"`
	Enabled     *bool   `json:"enabled,omitempty"`
	Description *string `json:"description,omitempty"`
}

// UpdateIOCRequest holds fields for updating an IOC.
type UpdateIOCRequest struct {
	Type        *IOCType `json:"type,omitempty"`
	Value       *string  `json:"value,omitempty"`
	Enabled     *bool    `json:"enabled,omitempty"`
	Description *string  `json:"description,omitempty"`
}

// IOCListOptions holds list filters and pagination.
type IOCListOptions struct {
	Type    *IOCType `json:"type,omitempty"`
	Enabled *bool    `json:"enabled,omitempty"`
	Search  *string  `json:"search,omitempty"` // matches value ILIKE '%search%'
	Limit   int      `json:"limit,omitempty"`
	Offset  int      `json:"offset,omitempty"`
}

// BulkCreateIOCsRequest supports textarea/file import; one IOC per line (value), plus defaults.
type BulkCreateIOCsRequest struct {
	Type        IOCType  `json:"type"`
	Values      []string `json:"values"`
	Enabled     *bool    `json:"enabled,omitempty"`
	Description *string  `json:"description,omitempty"`
}

// IOCLookupRequest represents a host lookup (can be domain or IP literal) from events.
type IOCLookupRequest struct {
	Host string `json:"host"`
}

func (r *IOCLookupRequest) Normalize() {
	r.Host = strings.TrimSuffix(strings.TrimSpace(strings.ToLower(r.Host)), ".")
}

func (r *IOCLookupRequest) Validate() error {
	if strings.TrimSpace(r.Host) == "" {
		return errors.New("host is required")
	}
	return nil
}

// ---- Validation ----

// Normalize applies canonical forms prior to persistence.
func (r *CreateIOCRequest) Normalize() {
	r.Type = IOCType(strings.TrimSpace(strings.ToLower(string(r.Type))))
	if r.Enabled == nil {
		b := true
		r.Enabled = &b
	}
	r.Value = canonicalIOCValue(r.Type, r.Value)
	if r.Description != nil {
		d := strings.TrimSpace(*r.Description)
		r.Description = &d
	}
}

func (r *CreateIOCRequest) Validate() error {
	if !r.Type.Valid() {
		return errors.New("invalid ioc type")
	}
	if strings.TrimSpace(r.Value) == "" {
		return errors.New("value is required")
	}
	if err := validateIOCValue(r.Type, r.Value); err != nil {
		return err
	}
	return nil
}

func (r *UpdateIOCRequest) Normalize() {
	if r.Type != nil {
		t := IOCType(strings.TrimSpace(strings.ToLower(string(*r.Type))))
		r.Type = &t
	}
	// Value canonicalization depends on resolved final type; defer to NormalizeWithFinalType
	if r.Description != nil {
		d := strings.TrimSpace(*r.Description)
		r.Description = &d
	}
}

// NormalizeWithFinalType canonicalizes Value using the resolved final type,
// where finalType is the existing type unless overridden by r.Type.
func (r *UpdateIOCRequest) NormalizeWithFinalType(finalType IOCType) {
	if r.Type != nil {
		t := IOCType(strings.TrimSpace(strings.ToLower(string(*r.Type))))
		r.Type = &t
	}
	if r.Value != nil {
		t := finalType
		if r.Type != nil {
			t = *r.Type
		}
		v := canonicalIOCValue(t, *r.Value)
		r.Value = &v
	}
	if r.Description != nil {
		d := strings.TrimSpace(*r.Description)
		r.Description = &d
	}
}

func (r *UpdateIOCRequest) Validate(finalType IOCType) error {
	if r.Type != nil && !r.Type.Valid() {
		return errors.New("invalid ioc type")
	}
	return r.validateOptionalValue(finalType)
}

// canonicalIOCValue returns a normalized storage value for the given type.
func canonicalIOCValue(t IOCType, raw string) string {
	raw = strings.TrimSpace(raw)
	switch t {
	case IOCTypeFQDN:
		s := strings.ToLower(raw)
		s = strings.TrimSuffix(s, ".")
		return s
	case IOCTypeIP:
		// Try CIDR first, then IP. If parseable, return canonical string; else return as-is (Validate will catch errors).
		if p, err := netip.ParsePrefix(raw); err == nil {
			return p.String()
		}
		if a, err := netip.ParseAddr(raw); err == nil {
			return a.String()
		}
		return raw
	default:
		return strings.ToLower(raw)
	}
}

func validateIOCValue(t IOCType, v string) error {
	switch t {
	case IOCTypeFQDN:
		return validateFQDNPattern(v)
	case IOCTypeIP:
		// Accept exact IP or CIDR prefix
		if _, err := netip.ParseAddr(v); err == nil {
			return nil
		}
		if _, err := netip.ParsePrefix(v); err == nil {
			return nil
		}
		return errors.New("invalid ip or cidr notation")
	default:
		return errors.New("unsupported ioc type")
	}
}

func (r *UpdateIOCRequest) validateOptionalValue(finalType IOCType) error {
	if r.Value == nil {
		return nil
	}

	if strings.TrimSpace(*r.Value) == "" {
		return errors.New("value cannot be empty")
	}

	t := finalType
	if r.Type != nil {
		t = *r.Type
	}

	return validateIOCValue(t, *r.Value)
}

// validateFQDNPattern validates domains and simple wildcard patterns.
// Allowed: exact like "evil.com"; wildcard labels like "*.evil.com" or "malware.*.com".
// Disallowed: empty labels, wildcard inside a label (e.g., "ma*ware.com").
func validateFQDNPattern(v string) error {
	v = strings.ToLower(strings.TrimSpace(v))
	if v == "" {
		return errors.New("domain is required")
	}
	labels := strings.Split(v, ".")
	if len(labels) < 2 {
		return errors.New("domain must contain a dot")
	}

	for i, lbl := range labels {
		if lbl == "" {
			return errors.New("domain contains empty label")
		}
		if lbl == "*" {
			// allow wildcard label but not for the TLD (last label)
			if i == len(labels)-1 {
				return errors.New("wildcard not allowed in TLD")
			}
			continue
		}
		if strings.Contains(lbl, "*") {
			return errors.New("wildcard must be a full label '*'")
		}
		if !domainLabelRe.MatchString(lbl) {
			return fmt.Errorf("invalid domain label: %q", lbl)
		}
	}
	return nil
}

// Normalize trims and clamps pagination/search inputs.
func (o *IOCListOptions) Normalize() {
	if o.Limit < 0 {
		o.Limit = 0
	}
	if o.Offset < 0 {
		o.Offset = 0
	}
	if o.Search != nil {
		s := strings.TrimSpace(*o.Search)
		o.Search = &s
	}
}

// Normalize canonicalizes and de-duplicates bulk values based on Type.
func (r *BulkCreateIOCsRequest) Normalize() {
	r.Type = IOCType(strings.TrimSpace(strings.ToLower(string(r.Type))))
	seen := map[string]struct{}{}
	out := make([]string, 0, len(r.Values))
	for _, v := range r.Values {
		c := canonicalIOCValue(r.Type, v)
		if c == "" {
			continue
		}
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		out = append(out, c)
	}
	r.Values = out
}
