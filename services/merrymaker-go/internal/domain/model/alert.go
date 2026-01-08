//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"
)

// Alert represents a fired security alert in the system.
type Alert struct {
	ID             string              `json:"id"                    db:"id"`
	SiteID         string              `json:"site_id"               db:"site_id"`
	RuleID         *string             `json:"rule_id,omitempty"     db:"rule_id"`
	RuleType       string              `json:"rule_type"             db:"rule_type"`
	Severity       string              `json:"severity"              db:"severity"`
	Title          string              `json:"title"                 db:"title"`
	Description    string              `json:"description"           db:"description"`
	EventContext   json.RawMessage     `json:"event_context"         db:"event_context"`
	Metadata       json.RawMessage     `json:"metadata,omitempty"    db:"metadata"`
	DeliveryStatus AlertDeliveryStatus `json:"delivery_status"       db:"delivery_status"`
	FiredAt        time.Time           `json:"fired_at"              db:"fired_at"`
	ResolvedAt     *time.Time          `json:"resolved_at,omitempty" db:"resolved_at"`
	ResolvedBy     *string             `json:"resolved_by,omitempty" db:"resolved_by"`
	CreatedAt      time.Time           `json:"created_at"            db:"created_at"`
}

// AlertRuleType represents the type of rule that triggered an alert.
type AlertRuleType string

const (
	AlertRuleTypeUnknownDomain AlertRuleType = "unknown_domain"
	AlertRuleTypeIOC           AlertRuleType = "ioc_domain"
	AlertRuleTypeYaraRule      AlertRuleType = "yara_rule"
	AlertRuleTypeCustom        AlertRuleType = "custom"
)

// Valid returns true if the alert rule type is valid.
func (t AlertRuleType) Valid() bool {
	switch t {
	case AlertRuleTypeUnknownDomain, AlertRuleTypeIOC, AlertRuleTypeYaraRule, AlertRuleTypeCustom:
		return true
	default:
		return false
	}
}

// String returns the string representation of the alert rule type.
func (t AlertRuleType) String() string {
	return string(t)
}

// AlertSeverity represents the severity level of an alert.
type AlertSeverity string

const (
	AlertSeverityCritical AlertSeverity = "critical"
	AlertSeverityHigh     AlertSeverity = "high"
	AlertSeverityMedium   AlertSeverity = "medium"
	AlertSeverityLow      AlertSeverity = "low"
	AlertSeverityInfo     AlertSeverity = "info"
)

// Valid returns true if the alert severity is valid.
func (s AlertSeverity) Valid() bool {
	switch s {
	case AlertSeverityCritical, AlertSeverityHigh, AlertSeverityMedium, AlertSeverityLow, AlertSeverityInfo:
		return true
	default:
		return false
	}
}

// String returns the string representation of the alert severity.
func (s AlertSeverity) String() string {
	return string(s)
}

// AlertDeliveryStatus tracks whether an alert was dispatched externally.
type AlertDeliveryStatus string

const (
	AlertDeliveryStatusPending    AlertDeliveryStatus = "pending"
	AlertDeliveryStatusMuted      AlertDeliveryStatus = "muted"
	AlertDeliveryStatusDispatched AlertDeliveryStatus = "dispatched"
	AlertDeliveryStatusFailed     AlertDeliveryStatus = "failed"
)

// Valid returns true when the delivery status is one of the supported values.
func (s AlertDeliveryStatus) Valid() bool {
	switch s {
	case AlertDeliveryStatusPending, AlertDeliveryStatusMuted, AlertDeliveryStatusDispatched, AlertDeliveryStatusFailed:
		return true
	default:
		return false
	}
}

func normalizeAlertDeliveryStatus(v AlertDeliveryStatus) AlertDeliveryStatus {
	normalized := AlertDeliveryStatus(strings.ToLower(strings.TrimSpace(string(v))))
	if normalized == "" {
		return AlertDeliveryStatusPending
	}
	return normalized
}

// CreateAlertRequest represents a request to create a new alert.
type CreateAlertRequest struct {
	SiteID         string              `json:"site_id"`
	RuleID         *string             `json:"rule_id,omitempty"`
	RuleType       string              `json:"rule_type"`
	Severity       string              `json:"severity"`
	Title          string              `json:"title"`
	Description    string              `json:"description"`
	EventContext   json.RawMessage     `json:"event_context,omitempty"`
	Metadata       json.RawMessage     `json:"metadata,omitempty"`
	FiredAt        *time.Time          `json:"fired_at,omitempty"`
	DeliveryStatus AlertDeliveryStatus `json:"delivery_status,omitempty"`
}

// Normalize normalizes the CreateAlertRequest fields.
func (r *CreateAlertRequest) Normalize() {
	r.SiteID = strings.TrimSpace(r.SiteID)
	r.RuleType = strings.TrimSpace(r.RuleType)
	r.Severity = strings.TrimSpace(r.Severity)
	r.Title = strings.TrimSpace(r.Title)
	r.Description = strings.TrimSpace(r.Description)
	r.DeliveryStatus = AlertDeliveryStatus(strings.TrimSpace(string(r.DeliveryStatus)))
}

// Validate validates the CreateAlertRequest fields.
func (r *CreateAlertRequest) Validate() error {
	if r.SiteID == "" {
		return errors.New("site_id is required")
	}

	if !AlertRuleType(r.RuleType).Valid() {
		return errors.New("invalid rule_type")
	}

	if !AlertSeverity(r.Severity).Valid() {
		return errors.New("invalid severity")
	}

	if r.Title == "" {
		return errors.New("title is required")
	}
	if utf8.RuneCountInString(r.Title) > 255 {
		return errors.New("title cannot exceed 255 characters")
	}

	if r.Description == "" {
		return errors.New("description is required")
	}

	r.DeliveryStatus = normalizeAlertDeliveryStatus(r.DeliveryStatus)
	if !r.DeliveryStatus.Valid() {
		return errors.New("invalid delivery_status")
	}

	return nil
}

// AlertListOptions represents options for listing alerts.
type AlertListOptions struct {
	SiteID     *string `json:"site_id,omitempty"`
	RuleType   *string `json:"rule_type,omitempty"`
	Severity   *string `json:"severity,omitempty"`
	Unresolved bool    `json:"unresolved,omitempty"`
	Sort       string  `json:"sort,omitempty"` // Sort field: "fired_at", "severity", "created_at" (default: "fired_at")
	Dir        string  `json:"dir,omitempty"`  // Sort direction: "asc", "desc" (default: "desc")
	Limit      int     `json:"limit,omitempty"`
	Offset     int     `json:"offset,omitempty"`
}

// AlertWithSiteName represents an alert with the associated site name.
// This type is used for JOIN queries to avoid N+1 query patterns.
type AlertWithSiteName struct {
	Alert
	SiteName      string        `json:"site_name"       db:"site_name"`
	SiteAlertMode SiteAlertMode `json:"site_alert_mode" db:"site_alert_mode"`
}

// AlertListResult contains both the list of alerts and the total count.
// This allows efficient pagination by returning both values in a single query.
type AlertListResult struct {
	Alerts []*AlertWithSiteName
	Total  int
}

// AlertStats represents statistics about alerts in the system.
type AlertStats struct {
	Total      int `json:"total"`
	Critical   int `json:"critical"`
	High       int `json:"high"`
	Medium     int `json:"medium"`
	Low        int `json:"low"`
	Info       int `json:"info"`
	Unresolved int `json:"unresolved"`
}
