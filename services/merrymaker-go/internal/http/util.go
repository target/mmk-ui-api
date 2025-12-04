package httpx

import (
	"net/http"
	"strconv"
	"strings"
)

// validationErrorPatterns holds common validation error substrings to classify 400 vs 5xx.
// Keeping this at package scope avoids per-call allocations in isValidationError.

var validationErrorPatterns = []string{ //nolint:gochecknoglobals // read-only cache of patterns to avoid per-call allocations
	"is required and cannot be empty",
	"value is required and cannot be empty",
	"cannot be empty",
	"cannot exceed",
	"at least one field must be updated",
	"cannot contain empty",
	"must be a valid URL",
	"must use http or https scheme",
	"must have a valid host",
	"must be one of:",
	"must be between",
	"must be non-negative",
	"must be at least",
	"must start with",
	"contain only",
}

// parseIntQuery returns the integer value of a query param or a default.
// It is tolerant of missing/invalid values.
func parseIntQuery(r *http.Request, key string, def int) int {
	if v := r.URL.Query().Get(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return def
}

// ParseLimitOffset parses common pagination params and clamps to sane bounds.
// - defLimit: default limit when not specified
// - maxLimit: maximum allowed limit (values > maxLimit are clamped to maxLimit).
func ParseLimitOffset(r *http.Request, defLimit, maxLimit int) (int, int) {
	// Defensive: ensure maxLimit is at least 1 to avoid clamping to 0 or negatives
	if maxLimit < 1 {
		maxLimit = 1
	}

	lim := parseIntQuery(r, "limit", defLimit)
	off := parseIntQuery(r, "offset", 0)
	if lim < 1 {
		lim = 1
	}
	if lim > maxLimit {
		lim = maxLimit
	}
	if off < 0 {
		off = 0
	}
	return lim, off
}

// isValidationError checks for common validation error patterns to decide 400 vs 5xx.
// This is a stopgap until typed validation errors are adopted across services.
func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	for _, p := range validationErrorPatterns {
		if strings.Contains(msg, p) {
			return true
		}
	}
	return false
}
