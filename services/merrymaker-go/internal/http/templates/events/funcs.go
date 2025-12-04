package events

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"strconv"
	"strings"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// Deps contains the dependencies required to build event-related template helpers.
type Deps struct {
	Template **template.Template
}

// Funcs returns a template.FuncMap with helpers tailored to event rendering.
func Funcs(deps Deps) template.FuncMap {
	funcs := template.FuncMap{
		"eventsToJSON":             serializeEventsToJSON,
		"eventTypeCategory":        EventTypeCategory,
		"eventTemplateName":        eventTemplateName,
		"formatEventData":          FormatEventData,
		"parseEventData":           ParseEventData,
		"safeDeref":                SafeDeref,
		"hasEventFilters":          HasEventFilters,
		"extractNetworkURLFromMap": ExtractNetworkURLFromMap,
		"truncateURL120":           func(url string) string { return TruncateURL(url, 120) },
		"networkEventSubtype":      NetworkEventSubtype,
		"getMapValue":              GetMapValue,
		"httpStatusClass":          HTTPStatusClass,
		"isHTTPURL":                IsHTTPURL,
	}

	funcs["renderEventPartial"] = func(name string, data any) (template.HTML, error) {
		if deps.Template == nil || *deps.Template == nil {
			return "", errors.New("template not initialized")
		}
		var buf bytes.Buffer
		if err := (*deps.Template).ExecuteTemplate(&buf, name, data); err != nil {
			return "", err
		}
		// #nosec G203 - Rendered HTML originates from our trusted template set and varies only by data already escaped by html/template.
		return template.HTML(buf.String()), nil
	}

	return funcs
}

// serializeEventsToJSON serializes events to JSON with proper event_data handling.
// It ensures event_data (json.RawMessage) is embedded as an object, not a string.
// Returns template.JS to prevent double-escaping in script tags.
func serializeEventsToJSON(events any) (template.JS, error) {
	eventsSlice, ok := events.([]*model.Event)
	if !ok {
		// Fallback to regular JSON if not the expected type
		b, err := json.Marshal(events)
		if err != nil {
			return "", err
		}
		// #nosec G203 - This is JSON data from our database, marshaled by encoding/json.
		// It's placed in <script type="application/json"> tags for client-side parsing.
		return template.JS(b), nil
	}

	result := buildEventMapsWithParsedData(eventsSlice)
	b, err := json.Marshal(result)
	if err != nil {
		return "", err
	}
	// #nosec G203 - This is JSON data from our database, marshaled by encoding/json.
	// It's placed in <script type="application/json"> tags for client-side parsing.
	return template.JS(b), nil
}

// buildEventMapsWithParsedData converts Event structs to maps with parsed event_data.
func buildEventMapsWithParsedData(events []*model.Event) []map[string]any {
	result := make([]map[string]any, 0, len(events))
	for _, ev := range events {
		m := map[string]any{
			"id":             ev.ID,
			"session_id":     ev.SessionID,
			"source_job_id":  ev.SourceJobID,
			"event_type":     ev.EventType,
			"storage_key":    ev.StorageKey,
			"priority":       ev.Priority,
			"should_process": ev.ShouldProcess,
			"processed":      ev.Processed,
			"created_at":     ev.CreatedAt,
		}

		// Parse event_data from JSON to object
		if len(ev.EventData) > 0 {
			var eventData any
			if err := json.Unmarshal(ev.EventData, &eventData); err == nil {
				m["event_data"] = eventData
			} else {
				// If parsing fails, include as string
				m["event_data"] = string(ev.EventData)
			}
		}

		result = append(result, m)
	}
	return result
}

// EventTypeCategory classifies event types into broad categories for rendering.
// Returns one of: "screenshot", "worker_log", "job_failure", "network", "console", "security", "page", "action", "error", "other".
func EventTypeCategory(eventType string) string {
	if eventType == "" {
		return EventCategoryOther
	}

	t := strings.ToLower(eventType)

	// Define category checkers in priority order
	categoryCheckers := []struct {
		category string
		check    func(string) bool
	}{
		{"screenshot", isScreenshotEvent},
		{"job_failure", isJobFailureEvent},
		{"worker_log", isWorkerLogEvent},
		{"network", isNetworkEvent},
		{"console", isConsoleEvent},
		{"security", isSecurityEvent},
		{"page", isPageEvent},
		{"action", isActionEvent},
		{"error", isErrorEvent},
	}

	for _, checker := range categoryCheckers {
		if checker.check(t) {
			return checker.category
		}
	}

	return EventCategoryOther
}

// eventTemplateName returns the template name for the given category and section.
func eventTemplateName(category, section string) string {
	base := category
	switch category {
	case "job_failure":
		base = "job-failure"
	case "worker_log":
		base = "worker-log"
	case "action":
		base = "page"
	case "page", "screenshot", "network", "console", "security", "error", EventCategoryOther:
		// keep base as-is
	case "":
		base = EventCategoryOther
	default:
		// leave category unchanged; templates share the category name.
	}

	if base == "" {
		base = EventCategoryOther
	}

	return fmt.Sprintf("%s-event-%s", base, section)
}

const (
	EventCategoryOther   = "other"
	StatusClassSuccess   = "success"
	StatusClassRedirect  = "redirect"
	StatusClassClientErr = "client-error"
	StatusClassServerErr = "server-error"
	StatusClassInfo      = "info"
)

func isScreenshotEvent(t string) bool {
	return strings.Contains(t, "screenshot")
}

// IsScreenshotEvent reports whether the event type should be treated as a screenshot event.
func IsScreenshotEvent(eventType string) bool {
	if eventType == "" {
		return false
	}
	return isScreenshotEvent(strings.ToLower(eventType))
}

func isJobFailureEvent(t string) bool {
	return strings.Contains(t, "jobfailure") || strings.Contains(t, "job.failure")
}

func isWorkerLogEvent(t string) bool {
	return t == "worker.log" || strings.HasSuffix(t, ".log")
}

func isNetworkEvent(t string) bool {
	return strings.Contains(t, "request") || strings.Contains(t, "response") ||
		strings.Contains(t, "network") || t == "network.loadingfailed"
}

func isConsoleEvent(t string) bool {
	return strings.Contains(t, "console") || t == "log"
}

func isSecurityEvent(t string) bool {
	return strings.HasPrefix(t, "security.") || strings.Contains(t, "dynamiccodeeval")
}

func isPageEvent(t string) bool {
	return strings.Contains(t, "goto") || strings.Contains(t, "navigate") ||
		strings.Contains(t, "page.goto")
}

func isActionEvent(t string) bool {
	actionTokens := []string{"click", "type", "waitforselector", "setcontent", "select", "hover"}
	for _, token := range actionTokens {
		if strings.Contains(t, token) {
			return true
		}
	}
	return false
}

func isErrorEvent(t string) bool {
	return strings.Contains(t, "error") || strings.Contains(t, "exception")
}

// IsErrorEvent reports whether the event type represents an error classification.
func IsErrorEvent(eventType string) bool {
	if eventType == "" {
		return false
	}
	return isErrorEvent(strings.ToLower(eventType))
}

// FormatEventData pretty-prints JSON data with indentation for display.
func FormatEventData(data json.RawMessage) string {
	if len(data) == 0 {
		return ""
	}

	var parsed any
	if err := json.Unmarshal(data, &parsed); err != nil {
		// If parsing fails, return as string
		return string(data)
	}

	formatted, err := json.MarshalIndent(parsed, "", "  ")
	if err != nil {
		return string(data)
	}

	return string(formatted)
}

// ParseEventData parses JSON event data into a map for template access.
func ParseEventData(data json.RawMessage) map[string]any {
	if len(data) == 0 {
		return map[string]any{}
	}

	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		return map[string]any{}
	}

	return parsed
}

// SafeDeref safely dereferences pointer fields to avoid nil pointer panics.
func SafeDeref(ptr any) string {
	if ptr == nil {
		return ""
	}

	return safeDerefValue(ptr)
}

func safeDerefValue(v any) string {
	switch val := v.(type) {
	case *string:
		if val != nil {
			return *val
		}
	case *int:
		if val != nil {
			return strconv.Itoa(*val)
		}
	case *int64:
		if val != nil {
			return strconv.FormatInt(*val, 10)
		}
	case *bool:
		if val != nil {
			return strconv.FormatBool(*val)
		}
	default:
		// For non-pointer types, convert to string
		return fmt.Sprintf("%v", val)
	}

	return ""
}

// ExtractNetworkURLFromMap extracts the URL from an already-parsed network event map.
// Handles both CDP Network.requestWillBeSent (request.url) and Network.responseReceived (response.url).
func ExtractNetworkURLFromMap(parsed map[string]any) string {
	// Try request.url first (Network.requestWillBeSent)
	if request, requestExists := parsed["request"].(map[string]any); requestExists {
		if url, urlFound := request["url"].(string); urlFound {
			return url
		}
	}

	// Try response.url (Network.responseReceived)
	if response, responseExists := parsed["response"].(map[string]any); responseExists {
		if url, urlFound := response["url"].(string); urlFound {
			return url
		}
	}

	// Try top-level url field
	if url, ok := parsed["url"].(string); ok {
		return url
	}

	return ""
}

// TruncateURL truncates a URL to a maximum length while preserving readability.
// Shows the beginning and end of the URL with ellipsis in the middle.
func TruncateURL(url string, maxLen int) string {
	if len(url) <= maxLen {
		return url
	}

	if maxLen < 20 {
		return url[:maxLen] + "..."
	}

	// Show first 60% and last 40% of the max length
	prefixLen := int(float64(maxLen) * 0.6)
	suffixLen := maxLen - prefixLen - 3 // 3 for "..."

	return url[:prefixLen] + "..." + url[len(url)-suffixLen:]
}

// NetworkEventSubtype extracts the subtype from a network event type.
func NetworkEventSubtype(eventType string) string {
	parts := strings.Split(eventType, ".")
	if len(parts) > 1 {
		return parts[1]
	}
	return eventType
}

// GetMapValue safely retrieves a value from a nested map structure.
func GetMapValue(data map[string]any, path string) any {
	parts := strings.Split(path, ".")
	current := data

	for i, part := range parts {
		if current == nil {
			return nil
		}

		val, exists := current[part]
		if !exists {
			return nil
		}

		if i == len(parts)-1 {
			return val
		}

		if nextMap, mapFound := val.(map[string]any); mapFound {
			current = nextMap
		} else {
			return nil
		}
	}

	return nil
}

// HTTPStatusClass returns a CSS class suffix based on HTTP status code.
func HTTPStatusClass(status any) string {
	code := ParseStatusCode(status)
	if code == 0 {
		return ""
	}

	switch {
	case code >= 200 && code < 300:
		return StatusClassSuccess
	case code >= 300 && code < 400:
		return StatusClassRedirect
	case code >= 400 && code < 500:
		return StatusClassClientErr
	case code >= 500:
		return StatusClassServerErr
	default:
		return StatusClassInfo
	}
}

func ParseStatusCode(status any) int {
	switch v := status.(type) {
	case int:
		return v
	case float64:
		return int(v)
	case string:
		if parsed, err := strconv.Atoi(v); err == nil {
			return parsed
		}
	}
	return 0
}

// HasEventFilters checks if any event filters are applied (template helper).
func HasEventFilters(filters any) bool {
	if filters == nil {
		return false
	}

	filterMap, ok := filters.(map[string]any)
	if !ok {
		return false
	}

	for _, key := range []string{"EventType", "Category", "SearchQuery", "SortBy", "SortDir"} {
		val, exists := filterMap[key]
		if !exists || val == nil {
			continue
		}

		if isActiveFilter(val) {
			return true
		}
	}

	return false
}

func isActiveFilter(val any) bool {
	strPtr, ok := val.(*string)
	if !ok || strPtr == nil {
		return false
	}

	return *strPtr != ""
}

// IsHTTPURL checks if a URL string starts with http:// or https:// (case-insensitive).
func IsHTTPURL(url string) bool {
	if url == "" {
		return false
	}
	lower := strings.ToLower(url)
	return strings.HasPrefix(lower, "http://") || strings.HasPrefix(lower, "https://")
}
