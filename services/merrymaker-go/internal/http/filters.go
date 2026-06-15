package httpx

import (
	"net/url"
	"strings"
)

// BaseFilter holds common filter fields shared across list pages.
// Embed this struct in page-specific filters to avoid duplicating parsing logic.
type BaseFilter struct {
	Q       string
	Enabled *bool
	Sort    string
	Dir     string
}

// ParseBaseFilter parses common fields from URL query parameters.
// Callers should embed the result and then override/extend with page-specific fields.
func ParseBaseFilter(q url.Values) BaseFilter {
	var enabledPtr *bool
	switch strings.TrimSpace(q.Get("enabled")) {
	case StrTrue, "1":
		b := true
		enabledPtr = &b
	case StrFalse, "0":
		b := false
		enabledPtr = &b
	}
	sort, dir := ParseSortParam(q, "sort", "dir")

	return BaseFilter{
		Q:       strings.TrimSpace(q.Get("q")),
		Enabled: enabledPtr,
		Sort:    sort,
		Dir:     dir,
	}
}

const (
	// StrTrue represents the string "true" for boolean query parameters.
	StrTrue = "true"
	// StrFalse represents the string "false" for boolean query parameters.
	StrFalse = "false"
	// SortDirAsc represents ascending sort direction.
	SortDirAsc = "asc"
	// SortDirDesc represents descending sort direction.
	SortDirDesc = "desc"
)

// ParseSortParam extracts and validates sort field and direction from URL query parameters.
// It supports two formats:
// 1. Combined format: ?sort=field:dir (e.g., ?sort=created_at:desc)
// 2. Separate format: ?sort=field&dir=direction (e.g., ?sort=created_at&dir=desc)
//
// The direction is normalized to lowercase and validated (must be "asc" or "desc").
// If the direction is invalid, it returns an empty string for dir.
//
// Parameters:
//   - q: URL query values
//   - sortKey: the query parameter key for the sort field (typically "sort")
//   - dirKey: the query parameter key for the direction (typically "dir")
//
// Returns the sort field name (trimmed) and the sort direction ("asc", "desc", or empty string if invalid).
func ParseSortParam(q url.Values, sortKey, dirKey string) (string, string) {
	sortParam := strings.TrimSpace(q.Get(sortKey))
	dirParam := strings.ToLower(strings.TrimSpace(q.Get(dirKey)))

	// Try to split on ":" first (avoids double allocation)
	parts := strings.SplitN(sortParam, ":", 2)
	if len(parts) == 2 {
		fieldPart := strings.TrimSpace(parts[0])
		dirPart := strings.ToLower(strings.TrimSpace(parts[1]))
		// Only accept known directions
		if dirPart == SortDirAsc || dirPart == SortDirDesc {
			return fieldPart, dirPart
		}
		// Invalid direction in colon syntax, return field only
		return fieldPart, ""
	}

	// Validate separate direction parameter
	if dirParam == SortDirAsc || dirParam == SortDirDesc {
		return sortParam, dirParam
	}

	// No valid direction found
	return sortParam, ""
}
