package util //nolint:revive // "util" is an established namespace for this project's shared HTTP-template helpers

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
