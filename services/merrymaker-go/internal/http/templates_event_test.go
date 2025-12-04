package httpx

import (
	"encoding/json"
	"testing"

	corefuncs "github.com/target/mmk-ui-api/internal/http/templates/core"
	eventfuncs "github.com/target/mmk-ui-api/internal/http/templates/events"
)

func TestEventTypeCategory(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty string", "", "other"},
		{"network request", "Network.requestWillBeSent", "network"},
		{"network response", "Network.responseReceived", "network"},
		{"network loading failed", "network.loadingfailed", "network"},
		{"console log", "Runtime.consoleAPICalled", "console"},
		{"console log simple", "log", "console"},
		{"security event", "Security.monitoringInitialized", "security"},
		{"dynamic code eval", "Runtime.dynamicCodeEval", "security"},
		{"page navigation", "Page.goto", "page"},
		{"screenshot", "Page.screenshot", "screenshot"},
		{"click action", "Page.click", "action"},
		{"type action", "Page.type", "action"},
		{"error event", "Runtime.exceptionThrown", "error"},
		{"unknown event", "SomeUnknownEvent", "other"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eventfuncs.EventTypeCategory(tt.input)
			if result != tt.expected {
				t.Errorf("eventTypeCategory(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestFormatEventData(t *testing.T) {
	tests := []struct {
		name     string
		input    json.RawMessage
		expected string
	}{
		{"empty data", json.RawMessage(""), ""},
		{"simple object", json.RawMessage(`{"key":"value"}`), "{\n  \"key\": \"value\"\n}"},
		{"invalid json", json.RawMessage(`{invalid`), "{invalid"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eventfuncs.FormatEventData(tt.input)
			if result != tt.expected {
				t.Errorf("formatEventData(%q) = %q, want %q", string(tt.input), result, tt.expected)
			}
		})
	}
}

func TestExtractNetworkURLFromMap(t *testing.T) {
	tests := []struct {
		name     string
		input    map[string]any
		expected string
	}{
		{
			name:     "empty data",
			input:    map[string]any{},
			expected: "",
		},
		{
			name: "requestWillBeSent with request.url",
			input: map[string]any{
				"request": map[string]any{"url": "https://example.com/path"},
			},
			expected: "https://example.com/path",
		},
		{
			name: "responseReceived with response.url",
			input: map[string]any{
				"response": map[string]any{"url": "https://example.com/api"},
			},
			expected: "https://example.com/api",
		},
		{
			name:     "top-level url field",
			input:    map[string]any{"url": "https://test.com"},
			expected: "https://test.com",
		},
		{
			name:     "no url field",
			input:    map[string]any{"method": "GET"},
			expected: "",
		},
		{
			name: "nil request",
			input: map[string]any{
				"request": nil,
			},
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eventfuncs.ExtractNetworkURLFromMap(tt.input)
			if result != tt.expected {
				t.Errorf("extractNetworkURLFromMap() = %q, want %q", result, tt.expected)
			}
		})
	}
}

func TestTruncateURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		maxLen   int
		expected string
	}{
		{
			name:     "short url no truncation",
			url:      "https://example.com",
			maxLen:   100,
			expected: "https://example.com",
		},
		{
			name:     "long url truncated",
			url:      "https://example.com/very/long/path/with/many/segments/that/should/be/truncated",
			maxLen:   50,
			expected: "https://example.com/very/long/...ould/be/truncated",
		},
		{
			name:     "very short maxLen",
			url:      "https://example.com/path",
			maxLen:   10,
			expected: "https://ex...",
		},
		{
			name:     "exact length",
			url:      "https://example.com",
			maxLen:   19,
			expected: "https://example.com",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eventfuncs.TruncateURL(tt.url, tt.maxLen)
			if result != tt.expected {
				t.Errorf("truncateURL(%q, %d) = %q, want %q", tt.url, tt.maxLen, result, tt.expected)
			}
		})
	}
}

func TestNetworkEventSubtype(t *testing.T) {
	tests := []struct {
		name      string
		eventType string
		expected  string
	}{
		{
			name:      "requestWillBeSent",
			eventType: "Network.requestWillBeSent",
			expected:  "requestWillBeSent",
		},
		{
			name:      "responseReceived",
			eventType: "Network.responseReceived",
			expected:  "responseReceived",
		},
		{
			name:      "loadingFailed",
			eventType: "Network.loadingFailed",
			expected:  "loadingFailed",
		},
		{
			name:      "no dot separator",
			eventType: "SomeEvent",
			expected:  "SomeEvent",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eventfuncs.NetworkEventSubtype(tt.eventType)
			if result != tt.expected {
				t.Errorf("networkEventSubtype(%q) = %q, want %q", tt.eventType, result, tt.expected)
			}
		})
	}
}

func TestGetMapValue(t *testing.T) {
	tests := []struct {
		name     string
		data     map[string]any
		path     string
		expected any
	}{
		{
			name:     "simple key",
			data:     map[string]any{"key": "value"},
			path:     "key",
			expected: "value",
		},
		{
			name:     "nested key",
			data:     map[string]any{"request": map[string]any{"url": "https://example.com"}},
			path:     "request.url",
			expected: "https://example.com",
		},
		{
			name:     "deeply nested",
			data:     map[string]any{"a": map[string]any{"b": map[string]any{"c": 123}}},
			path:     "a.b.c",
			expected: 123,
		},
		{
			name:     "missing key",
			data:     map[string]any{"key": "value"},
			path:     "missing",
			expected: nil,
		},
		{
			name:     "missing nested key",
			data:     map[string]any{"request": map[string]any{"method": "GET"}},
			path:     "request.url",
			expected: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eventfuncs.GetMapValue(tt.data, tt.path)
			if result != tt.expected {
				t.Errorf("getMapValue(%v, %q) = %v, want %v", tt.data, tt.path, result, tt.expected)
			}
		})
	}
}

func TestHTTPStatusClass(t *testing.T) {
	tests := []struct {
		name     string
		status   any
		expected string
	}{
		{name: "200 success", status: 200, expected: eventfuncs.StatusClassSuccess},
		{name: "201 success", status: 201, expected: eventfuncs.StatusClassSuccess},
		{name: "301 redirect", status: 301, expected: eventfuncs.StatusClassRedirect},
		{name: "404 client error", status: 404, expected: eventfuncs.StatusClassClientErr},
		{name: "500 server error", status: 500, expected: eventfuncs.StatusClassServerErr},
		{name: "100 info", status: 100, expected: eventfuncs.StatusClassInfo},
		{name: "float64 200", status: float64(200), expected: eventfuncs.StatusClassSuccess},
		{name: "string 404", status: "404", expected: eventfuncs.StatusClassClientErr},
		{name: "invalid string", status: "invalid", expected: ""},
		{name: "nil", status: nil, expected: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eventfuncs.HTTPStatusClass(tt.status)
			if result != tt.expected {
				t.Errorf("httpStatusClass(%v) = %q, want %q", tt.status, result, tt.expected)
			}
		})
	}
}

func TestStrLen(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected int
	}{
		{name: "empty string", input: "", expected: 0},
		{name: "short string", input: "hello", expected: 5},
		{name: "long url", input: "https://example.com/very/long/path", expected: 34},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := corefuncs.StrLen(tt.input)
			if result != tt.expected {
				t.Errorf("strLen(%q) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestSafeDeref(t *testing.T) {
	stringVal := "test"
	intVal := 42
	int64Val := int64(123)
	boolVal := true

	tests := []struct {
		name     string
		input    any
		expected string
	}{
		{"nil", nil, ""},
		{"string pointer", &stringVal, "test"},
		{"nil string pointer", (*string)(nil), ""},
		{"int pointer", &intVal, "42"},
		{"nil int pointer", (*int)(nil), ""},
		{"int64 pointer", &int64Val, "123"},
		{"nil int64 pointer", (*int64)(nil), ""},
		{"bool pointer", &boolVal, "true"},
		{"nil bool pointer", (*bool)(nil), ""},
		{"non-pointer string", "direct", "direct"},
		{"non-pointer int", 99, "99"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := eventfuncs.SafeDeref(tt.input)
			if result != tt.expected {
				t.Errorf("safeDeref(%v) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}
