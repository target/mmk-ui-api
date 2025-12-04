package uiutil

import (
	"strconv"
	"strings"
	"time"
)

const FriendlyDateTimeLayout = "Jan 2, 2006 3:04 PM"

// FriendlyRelativeTime returns a human-friendly description of how long ago t occurred.
// Times in the future are treated as "just now" to avoid confusing negative durations.
func FriendlyRelativeTime(t time.Time) string {
	diff := time.Since(t)
	if diff < 0 {
		return "just now"
	}

	switch {
	case diff < time.Minute:
		return "just now"
	case diff < time.Hour:
		mins := int(diff.Minutes())
		if mins == 1 {
			return "1 minute ago"
		}
		return strconv.Itoa(mins) + " minutes ago"
	case diff < 24*time.Hour:
		hours := int(diff.Hours())
		if hours == 1 {
			return "1 hour ago"
		}
		return strconv.Itoa(hours) + " hours ago"
	case diff < 7*24*time.Hour:
		days := int(diff.Hours() / 24)
		if days == 1 {
			return "1 day ago"
		}
		return strconv.Itoa(days) + " days ago"
	default:
		return FormatFriendlyDateTime(t)
	}
}

// FormatFriendlyDateTime returns a consistent, user-friendly local timestamp representation.
func FormatFriendlyDateTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Local().Format(FriendlyDateTimeLayout)
}

// TruncateWithEllipsis shortens text to the provided rune limit and appends an ellipsis when truncated.
func TruncateWithEllipsis(text string, limit int) string {
	runes := []rune(text)
	if len(runes) <= limit {
		return text
	}
	if limit <= 1 {
		return "…"
	}
	return strings.TrimSpace(string(runes[:limit-1])) + "…"
}
