package util //nolint:revive // package name util hosts shared formatting helpers used across HTTP templates

import "time"

// FormatProcessingDuration formats a time.Duration for display, handling edge cases.
// Returns "—" for zero or negative durations, truncates to milliseconds for readability.
func FormatProcessingDuration(d time.Duration) string {
	switch {
	case d <= 0:
		return "—"
	case d < time.Millisecond:
		return d.String()
	default:
		return d.Truncate(time.Millisecond).String()
	}
}
