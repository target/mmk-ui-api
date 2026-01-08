package service

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// AlertDeliveryJobResult captures a single delivery attempt result for an alert sink job.
// Stored as JSON in the job_results table so the UI can render history even after jobs are reaped.
type AlertDeliveryJobResult struct {
	JobID         string                      `json:"job_id"`
	AlertID       string                      `json:"alert_id,omitempty"`
	SinkID        string                      `json:"sink_id"`
	SinkName      string                      `json:"sink_name,omitempty"`
	JobStatus     model.JobStatus             `json:"job_status"`
	AttemptNumber int                         `json:"attempt_number"`
	RetryCount    int                         `json:"retry_count,omitempty"`
	MaxRetries    int                         `json:"max_retries,omitempty"`
	AttemptedAt   time.Time                   `json:"attempted_at"`
	CompletedAt   *time.Time                  `json:"completed_at,omitempty"`
	DurationMs    int64                       `json:"duration_ms,omitempty"`
	Payload       json.RawMessage             `json:"payload,omitempty"`
	Request       AlertDeliveryRequestSummary `json:"request"`
	Response      *AlertDeliveryResponse      `json:"response,omitempty"`
	ErrorMessage  string                      `json:"error_message,omitempty"`
}

// AlertDeliveryRequestSummary records the HTTP request that was attempted, with secrets redacted.
type AlertDeliveryRequestSummary struct {
	Method        string            `json:"method"`
	URL           string            `json:"url"`
	Headers       map[string]string `json:"headers,omitempty"`
	Body          string            `json:"body,omitempty"`
	BodyTruncated bool              `json:"body_truncated,omitempty"`
	OkStatus      int               `json:"ok_status,omitempty"`
}

// AlertDeliveryResponse records the HTTP response payload.
type AlertDeliveryResponse struct {
	StatusCode    int               `json:"status_code"`
	Headers       map[string]string `json:"headers,omitempty"`
	Body          string            `json:"body,omitempty"`
	BodyTruncated bool              `json:"body_truncated,omitempty"`
}

// SecretRedactor replaces concrete secret values with their placeholder tokens to avoid persistence.
type SecretRedactor struct {
	secrets map[string]string
}

// NewSecretRedactor constructs a redactor for the provided secret map (placeholder -> secret value).
func NewSecretRedactor(secrets map[string]string) SecretRedactor {
	if len(secrets) == 0 {
		return SecretRedactor{}
	}
	clone := make(map[string]string, len(secrets))
	for k, v := range secrets {
		clone[k] = v
	}
	return SecretRedactor{secrets: clone}
}

// RedactString replaces secret values within the string with their placeholders.
func (r SecretRedactor) RedactString(s string) string {
	if len(r.secrets) == 0 || s == "" {
		return s
	}
	out := s
	for placeholder, value := range r.secrets {
		if value == "" || placeholder == "" {
			continue
		}
		candidates := map[string]struct{}{
			value: {},
		}
		if escaped := url.QueryEscape(value); escaped != "" {
			candidates[escaped] = struct{}{}
		}
		if pathEscaped := url.PathEscape(value); pathEscaped != "" {
			candidates[pathEscaped] = struct{}{}
		}
		if strings.Contains(value, " ") {
			candidates[strings.ReplaceAll(value, " ", "+")] = struct{}{}
		}
		for candidate := range candidates {
			out = strings.ReplaceAll(out, candidate, placeholder)
		}
	}
	return out
}

// RedactHeaders applies redaction for each header value.
// Additionally masks values for sensitive header keys (Authorization, API-Key, Token, etc.).
func (r SecretRedactor) RedactHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		redacted := r.RedactString(v)
		// Mask sensitive header values regardless of secret redaction
		if isSensitiveHeader(k) {
			out[k] = maskHeaderValue(redacted)
		} else {
			out[k] = redacted
		}
	}
	return out
}

// isSensitiveHeader checks if a header key contains sensitive information.
func isSensitiveHeader(key string) bool {
	lower := strings.ToLower(key)
	sensitiveKeys := []string{
		"authorization",
		"api-key",
		"apikey",
		"x-api-key",
		"x-apikey",
		"token",
		"x-token",
		"auth-token",
		"x-auth-token",
		"access-token",
		"x-access-token",
		"secret",
		"x-secret",
		"password",
		"passwd",
		"credential",
		"cookie",
		"set-cookie",
		"session",
		"x-session",
		"private-token",
		"x-private-token",
	}
	for _, sensitive := range sensitiveKeys {
		if strings.Contains(lower, sensitive) {
			return true
		}
	}
	return false
}

// maskHeaderValue masks a header value while showing a hint of its structure.
// Examples: "Bearer ***" for "Bearer token123", "***" for short values.
func maskHeaderValue(value string) string {
	if value == "" {
		return ""
	}

	// Already redacted with a placeholder (e.g., __SECRET_NAME__)
	if strings.HasPrefix(value, "__") && strings.HasSuffix(value, "__") {
		return value
	}

	// For Bearer tokens, preserve the "Bearer" prefix
	if strings.HasPrefix(value, "Bearer ") || strings.HasPrefix(value, "bearer ") {
		return value[:7] + "***"
	}

	// For Basic auth, preserve the "Basic" prefix
	if strings.HasPrefix(value, "Basic ") || strings.HasPrefix(value, "basic ") {
		return value[:6] + "***"
	}

	// For values with spaces, show prefix and mask the rest
	if idx := strings.Index(value, " "); idx > 0 && idx < 20 {
		return value[:idx+1] + "***"
	}

	// For short values (< 8 chars), completely mask
	if len(value) <= 8 {
		return "***"
	}

	// For longer values, show first few chars
	return value[:3] + "***"
}
