package alerts

import (
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
	"github.com/target/mmk-ui-api/internal/http/ui/viewmodel"
	"github.com/target/mmk-ui-api/internal/http/uiutil"
)

// AlertRow represents a single alert entry rendered in list and detail views.
type AlertRow struct {
	ID             string
	SiteID         string
	SiteName       string
	SiteAlertMode  model.SiteAlertMode
	DeliveryStatus model.AlertDeliveryStatus
	RuleType       string
	Severity       string
	Title          string
	Description    string
	JobDetails     []MetadataDetail
	FiredAt        time.Time
	ResolvedAt     *time.Time
	ResolvedBy     *string
	IsResolved     bool
}

// DeliveryRow represents a single delivery attempt for an alert.
type DeliveryRow struct {
	SinkID        string
	SinkName      string
	JobID         string
	Status        string
	AttemptNumber int
	ErrorMessage  string
	AttemptedAt   time.Time
	CompletedAt   *time.Time
}

// DeliveryStats aggregates delivery attempt counts by status.
type DeliveryStats struct {
	Total     int
	Completed int
	Failed    int
	Running   int
	Pending   int
	Other     int
}

// FriendlyFiredAt renders a human-friendly timestamp for when the alert fired.
func (r AlertRow) FriendlyFiredAt() string {
	return uiutil.FriendlyRelativeTime(r.FiredAt)
}

// StatusBadgeClass returns the CSS modifier class for the alert status badge.
func (r AlertRow) StatusBadgeClass() string {
	if r.IsResolved {
		return "badge-success"
	}
	return "badge-warning"
}

// WasMutedOnFire reports whether the alert delivery was muted when it was created.
func (r AlertRow) WasMutedOnFire() bool {
	return r.DeliveryStatus == model.AlertDeliveryStatusMuted
}

// MutedOnFireLabel returns the accessible label describing the muted-on-fire state.
func (r AlertRow) MutedOnFireLabel() string {
	return "Alert was muted when it fired"
}

// DeliveryStatusBadgeClass returns the CSS class to render the delivery status badge.
func (r AlertRow) DeliveryStatusBadgeClass() string {
	switch r.DeliveryStatus {
	case model.AlertDeliveryStatusMuted:
		return "badge-warning"
	case model.AlertDeliveryStatusDispatched:
		return "badge-success"
	case model.AlertDeliveryStatusFailed:
		return "badge-danger"
	case model.AlertDeliveryStatusPending:
		return "badge-info"
	default:
		return "badge-secondary"
	}
}

// DeliveryStatusIcon returns the icon identifier associated with the delivery status.
func (r AlertRow) DeliveryStatusIcon() string {
	switch r.DeliveryStatus {
	case model.AlertDeliveryStatusMuted:
		return "bell-off"
	case model.AlertDeliveryStatusDispatched:
		return "check-circle"
	case model.AlertDeliveryStatusFailed:
		return "x-circle"
	case model.AlertDeliveryStatusPending:
		return "loader"
	default:
		return "help-circle"
	}
}

// DeliveryStatusDisplay returns a human-readable label for the delivery status.
func (r AlertRow) DeliveryStatusDisplay() string {
	switch r.DeliveryStatus {
	case model.AlertDeliveryStatusMuted:
		return "Muted"
	case model.AlertDeliveryStatusDispatched:
		return "Dispatched"
	case model.AlertDeliveryStatusFailed:
		return "Failed"
	case model.AlertDeliveryStatusPending:
		return "Pending"
	default:
		return "Unknown"
	}
}

// SiteAlertModeStr returns the normalized site alert mode string.
func (r AlertRow) SiteAlertModeStr() string {
	mode := r.SiteAlertMode
	if !mode.Valid() {
		mode = model.SiteAlertModeActive
	}
	return string(mode)
}

// SiteAlertModeDisplay returns a human-readable label for the site alert mode.
func (r AlertRow) SiteAlertModeDisplay() string {
	if r.IsSiteMuted() {
		return "Muted"
	}
	return "Active"
}

// IsSiteMuted reports whether the alert's site is operating in muted mode.
func (r AlertRow) IsSiteMuted() bool {
	return r.SiteAlertMode == model.SiteAlertModeMuted
}

// PrimaryLabel returns the main piece of text to show for the alert.
// For domain-based alerts, this prioritizes the domain; otherwise falls back to the title.
func (r AlertRow) PrimaryLabel() string {
	if summary := r.ContextSummary(); summary != "" {
		return summary
	}
	return r.Title
}

// RuleTypeDisplay returns a user-friendly label for the rule type.
func (r AlertRow) RuleTypeDisplay() string {
	return ruleTypeDisplay(r.RuleType)
}

// ContextSummary returns a condensed description of the alert context for list displays.
func (r AlertRow) ContextSummary() string {
	return summarizeAlertContext(r.RuleType, r.Description)
}

// SiteOption represents a selectable site in the filter controls.
type SiteOption struct {
	ID   string
	Name string
}

// DetailPage represents the data required to render the alert detail view.
type DetailPage struct {
	viewmodel.Layout

	Alert               *AlertRow
	JobDetails          []MetadataDetail
	FiredAtFormatted    string
	ResolvedAtFormatted string
	EventContextDisplay string
	MetadataDisplay     string
	JobID               string
	RuleID              string

	Deliveries    []DeliveryRow
	DeliveryStats DeliveryStats

	SinksConfigured bool
	JobsMayBeReaped bool

	Error        bool
	ErrorMessage string
}

// LayoutData returns a pointer to the embedded layout for renderer helpers.
func (p *DetailPage) LayoutData() *viewmodel.Layout {
	return &p.Layout
}

// Page is the typed view model passed to the alerts list template.
type Page struct {
	viewmodel.Layout
	viewmodel.Pagination

	Alerts          []AlertRow
	SiteOptions     []SiteOption
	SeverityOptions []string
	RuleTypeOptions []string

	SiteID     string
	Severity   string
	RuleType   string
	Unresolved bool
	Sort       string
	Dir        string

	Error        bool
	ErrorMessage string
}

// LayoutData returns a pointer to the embedded layout for renderer helpers.
func (p *Page) LayoutData() *viewmodel.Layout {
	return &p.Layout
}

// MetadataDetail represents a key/value pair derived from alert metadata for display.
type MetadataDetail struct {
	Label string
	Value string
}

func summarizeAlertContext(ruleType, description string) string {
	desc := strings.TrimSpace(description)
	if desc == "" {
		return ""
	}

	switch strings.ToLower(ruleType) {
	case "unknown_domain":
		if domain := extractDomainFromUnknownDescription(desc); domain != "" {
			return domain
		}
	case "ioc_domain":
		if domain := extractDomainFromIOCString(desc); domain != "" {
			return domain
		}
	}

	return uiutil.TruncateWithEllipsis(desc, 90)
}

func extractDomainFromUnknownDescription(desc string) string {
	const prefix = "First time seen domain:"
	rest, ok := trimPrefixCaseInsensitive(desc, prefix)
	if !ok {
		return ""
	}
	return normalizeDomainCandidate(rest)
}

func extractDomainFromIOCString(desc string) string {
	prefixes := []string{
		"IOC domain matched:",
		"Known IOC detected:",
	}
	for _, prefix := range prefixes {
		if rest, ok := trimPrefixCaseInsensitive(desc, prefix); ok {
			return normalizeDomainCandidate(rest)
		}
	}
	return ""
}

func ruleTypeDisplay(ruleType string) string {
	switch model.AlertRuleType(ruleType) {
	case model.AlertRuleTypeUnknownDomain:
		return "Unknown Domain"
	case model.AlertRuleTypeIOC:
		return "IOC Domain"
	case model.AlertRuleTypeYaraRule:
		return "YARA Rule"
	case model.AlertRuleTypeCustom:
		return "Custom Rule"
	default:
		return humanizeIdentifier(ruleType)
	}
}

func humanizeIdentifier(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}

	parts := strings.FieldsFunc(value, func(r rune) bool {
		return r == '_' || r == '-' || r == '/'
	})
	for i, part := range parts {
		if part == "" {
			continue
		}
		parts[i] = strings.ToUpper(part[:1]) + strings.ToLower(part[1:])
	}
	return strings.Join(parts, " ")
}

func trimPrefixCaseInsensitive(text, prefix string) (string, bool) {
	candidates := []string{prefix}
	if base := strings.TrimSuffix(prefix, ":"); base != prefix {
		candidates = append(candidates, base)
	}

	desc := strings.TrimSpace(text)
	if desc == "" {
		return "", false
	}

	descLower := strings.ToLower(desc)
	for _, candidate := range candidates {
		candidateLower := strings.ToLower(candidate)
		if strings.HasPrefix(descLower, candidateLower) {
			rest := strings.TrimSpace(desc[len(candidate):])
			rest = strings.TrimLeft(rest, ":;-")
			rest = strings.TrimSpace(rest)
			if rest == "" {
				return "", false
			}
			return rest, true
		}
	}
	return "", false
}

func normalizeDomainCandidate(value string) string {
	v := strings.TrimSpace(value)
	if v == "" {
		return ""
	}
	if idx := strings.Index(v, "("); idx > -1 {
		v = v[:idx]
	}
	v = strings.TrimSpace(v)
	v = strings.Trim(v, " .,!;:-")
	if space := strings.Index(v, " "); space > -1 {
		v = strings.TrimSpace(v[:space])
	}
	v = strings.Trim(v, " .,!;:-")
	return v
}
