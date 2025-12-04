package service

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/target/mmk-ui-api/internal/domain/model"
	domainrules "github.com/target/mmk-ui-api/internal/domain/rules"
)

func TestNewEventFilterService(t *testing.T) {
	service := NewEventFilterService()

	assert.NotNil(t, service)
	assert.NotEmpty(t, service.ProcessableEventTypes)

	// Check that default processable event types are set
	expectedTypes := []string{
		"Network.requestWillBeSent",
		"Network.responseReceived",
	}

	for _, eventType := range expectedTypes {
		assert.True(
			t,
			service.ProcessableEventTypes[eventType],
			"Event type %s should be processable by default",
			eventType,
		)
	}
}

func TestEventFilterService_ShouldProcessEvent(t *testing.T) {
	service := NewEventFilterService()

	tests := []struct {
		name      string
		eventType string
		expected  bool
	}{
		{
			name:      "network request event",
			eventType: "Network.requestWillBeSent",
			expected:  true,
		},
		{
			name:      "network response event",
			eventType: "Network.responseReceived",
			expected:  true,
		},
		{
			name:      "domain seen event (not processable by default)",
			eventType: "domain_seen",
			expected:  false,
		},
		{
			name:      "file seen event (not processable by default)",
			eventType: "file_seen",
			expected:  false,
		},
		{
			name:      "unknown event type",
			eventType: "unknown_event",
			expected:  false,
		},
		{
			name:      "console log event",
			eventType: "Runtime.consoleAPICalled",
			expected:  false,
		},
		{
			name:      "case insensitive (domain_seen) not processable",
			eventType: "DOMAIN_SEEN",
			expected:  false,
		},
		{
			name:      "whitespace trimming (domain_seen) not processable",
			eventType: "  domain_seen  ",
			expected:  false,
		},
		{
			name:      "empty event type",
			eventType: "",
			expected:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := service.ShouldProcessEvent(tt.eventType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestEventFilterService_ShouldProcessEvents(t *testing.T) {
	service := NewEventFilterService()

	events := []model.RawEvent{
		{
			Type: "Network.requestWillBeSent",
			Data: json.RawMessage(`{"url":"https://example.com"}`),
		},
		{
			Type: "Runtime.consoleAPICalled",
			Data: json.RawMessage(`{"type":"log"}`),
		},
		{
			Type: "domain_seen",
			Data: json.RawMessage(`{"domain":"example.com"}`),
		},
		{
			Type: "unknown_event",
			Data: json.RawMessage(`{}`),
		},
	}

	result := service.ShouldProcessEvents(events)

	assert.Len(t, result, 4)
	assert.True(t, result[0])  // Network.requestWillBeSent should be processed
	assert.False(t, result[1]) // Runtime.consoleAPICalled should not be processed
	assert.False(t, result[2]) // domain_seen should NOT be processed by default
	assert.False(t, result[3]) // unknown_event should not be processed
}

func TestEventFilterService_FilterProcessableEvents(t *testing.T) {
	service := NewEventFilterService()

	events := []model.RawEvent{
		{
			Type: "Network.requestWillBeSent",
			Data: json.RawMessage(`{"url":"https://example.com"}`),
		},
		{
			Type: "Runtime.consoleAPICalled",
			Data: json.RawMessage(`{"type":"log"}`),
		},
		{
			Type: "domain_seen",
			Data: json.RawMessage(`{"domain":"example.com"}`),
		},
		{
			Type: "file_seen",
			Data: json.RawMessage(`{"hash":"abc123"}`),
		},
	}

	processableEvents := service.FilterProcessableEvents(events)

	assert.Len(t, processableEvents, 1)
	assert.Equal(t, "Network.requestWillBeSent", processableEvents[0].Type)
}

func TestEventFilterService_AddRemoveProcessableEventType(t *testing.T) {
	service := NewEventFilterService()

	// Add a new processable event type
	service.AddProcessableEventType("custom_event")
	assert.True(t, service.ShouldProcessEvent("custom_event"))

	// Remove an existing processable event type
	service.RemoveProcessableEventType("domain_seen")
	assert.False(t, service.ShouldProcessEvent("domain_seen"))

	// Add back the removed event type
	service.SetProcessableEventType("domain_seen", true)
	assert.True(t, service.ShouldProcessEvent("domain_seen"))

	// Disable an event type without removing it
	service.SetProcessableEventType("domain_seen", false)
	assert.False(t, service.ShouldProcessEvent("domain_seen"))
}

func TestEventFilterService_GetProcessableEventTypes(t *testing.T) {
	service := NewEventFilterService()

	eventTypes := service.GetProcessableEventTypes()

	// Should return a copy, not the original map
	assert.NotSame(t, &service.ProcessableEventTypes, &eventTypes)

	// Should contain the same data
	assert.Len(t, eventTypes, len(service.ProcessableEventTypes))
	for eventType, shouldProcess := range service.ProcessableEventTypes {
		assert.Equal(t, shouldProcess, eventTypes[eventType])
	}
}

func TestEventFilterService_GetProcessableEventTypesList(t *testing.T) {
	service := NewEventFilterService()

	// Add a disabled event type
	service.SetProcessableEventType("disabled_event", false)

	eventTypesList := service.GetProcessableEventTypesList()

	// Should only contain enabled event types
	for _, eventType := range eventTypesList {
		assert.True(t, service.ProcessableEventTypes[eventType], "Event type %s should be enabled", eventType)
	}

	// Should not contain disabled event types
	assert.NotContains(t, eventTypesList, "disabled_event")
}

func TestEventFilterService_GetFilterStats(t *testing.T) {
	service := NewEventFilterService()

	events := []model.RawEvent{
		{Type: "Network.requestWillBeSent"},
		{Type: "Runtime.consoleAPICalled"},
		{Type: "domain_seen"},
		{Type: "unknown_event"},
		{Type: "file_seen"},
	}

	stats := service.GetFilterStats(events)

	assert.Equal(t, 5, stats.TotalEvents)
	assert.Equal(t, 1, stats.ProcessableEvents)      // Only Network.requestWillBeSent
	assert.Equal(t, 4, stats.FilteredEvents)         // Others filtered
	assert.InDelta(t, 80.0, stats.FilterRatio, 0.01) // 4/5 * 100 = 80%
}

func TestEventFilterService_GetFilterStats_EmptyEvents(t *testing.T) {
	service := NewEventFilterService()

	events := []model.RawEvent{}

	stats := service.GetFilterStats(events)

	assert.Equal(t, 0, stats.TotalEvents)
	assert.Equal(t, 0, stats.ProcessableEvents)
	assert.Equal(t, 0, stats.FilteredEvents)
	assert.InDelta(t, 0.0, stats.FilterRatio, 0.01)
}

func TestNetworkEventExtractor(t *testing.T) {
	extractor := domainrules.NewNetworkEventExtractor()

	t.Run("network event domain extraction", func(t *testing.T) {
		event := model.RawEvent{
			Type: "Network.requestWillBeSent",
			Data: json.RawMessage(`{"request":{"url":"https://example.com/path"}}`),
		}

		domain, ok := extractor.ExtractDomainFromNetworkEvent(event)

		assert.True(t, ok)
		assert.Equal(t, "example.com", domain)
	})

	t.Run("non-network event", func(t *testing.T) {
		event := model.RawEvent{
			Type: "domain_seen",
			Data: json.RawMessage(`{"domain":"example.com"}`),
		}

		domain, ok := extractor.ExtractDomainFromNetworkEvent(event)

		assert.False(t, ok)
		assert.Empty(t, domain)
	})

	t.Run("file event hash extraction", func(t *testing.T) {
		event := model.RawEvent{
			Type: "file_seen",
			Data: json.RawMessage(`{"hash":"abc123def456"}`),
		}

		hash, ok := extractor.ExtractFileHashFromFileEvent(event)

		// Not a valid sha256 (not 64 hex chars)
		assert.False(t, ok)
		assert.Empty(t, hash)
	})

	t.Run("non-file event", func(t *testing.T) {
		event := model.RawEvent{
			Type: "Network.requestWillBeSent",
			Data: json.RawMessage(`{"url":"https://example.com"}`),
		}

		hash, ok := extractor.ExtractFileHashFromFileEvent(event)

		assert.False(t, ok)
		assert.Empty(t, hash)
	})

	t.Run("file event valid sha256 extraction", func(t *testing.T) {
		event := model.RawEvent{
			Type: "file_seen",
			Data: json.RawMessage(`{"sha256":"0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"}`),
		}
		hash, ok := extractor.ExtractFileHashFromFileEvent(event)
		assert.True(t, ok)
		assert.Equal(t, "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef", hash)
	})

	t.Run("domain extraction without scheme", func(t *testing.T) {
		event := model.RawEvent{
			Type: "Network.requestWillBeSent",
			Data: json.RawMessage(`{"url":"example.com/path"}`),
		}
		domain, ok := extractor.ExtractDomainFromNetworkEvent(event)
		assert.True(t, ok)
		assert.Equal(t, "example.com", domain)
	})

	t.Run("domain extraction IPv6 with port", func(t *testing.T) {
		event := model.RawEvent{
			Type: "Network.requestWillBeSent",
			Data: json.RawMessage(`{"request":{"url":"http://[2001:db8::1]:8443/path"}}`),
		}
		domain, ok := extractor.ExtractDomainFromNetworkEvent(event)
		assert.True(t, ok)
		assert.Equal(t, "2001:db8::1", domain)
	})

	t.Run("domain extraction host with port", func(t *testing.T) {
		event := model.RawEvent{
			Type: "Network.responseReceived",
			Data: json.RawMessage(`{"response":{"url":"https://example.com:8443/path"}}`),
		}
		domain, ok := extractor.ExtractDomainFromNetworkEvent(event)
		assert.True(t, ok)
		assert.Equal(t, "example.com", domain)
	})

	t.Run("file event uppercase sha256 lowercased", func(t *testing.T) {
		upper := "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"
		event := model.RawEvent{Type: "file_seen", Data: json.RawMessage(`{"sha256":"` + upper + `"}`)}
		hash, ok := extractor.ExtractFileHashFromFileEvent(event)
		assert.True(t, ok)
		assert.Equal(t, "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", hash)
	})
}

func TestEventFilterService_Integration(t *testing.T) {
	service := NewEventFilterService()

	// Simulate a realistic batch of events from puppeteer
	events := []model.RawEvent{
		{
			Type:      "Network.requestWillBeSent",
			Data:      json.RawMessage(`{"request":{"url":"https://example.com/api"}}`),
			Timestamp: time.Now(),
		},
		{
			Type:      "Network.responseReceived",
			Data:      json.RawMessage(`{"response":{"status":200}}`),
			Timestamp: time.Now(),
		},
		{
			Type:      "Runtime.consoleAPICalled",
			Data:      json.RawMessage(`{"type":"log","args":["Hello World"]}`),
			Timestamp: time.Now(),
		},
		{
			Type:      "domain_seen",
			Data:      json.RawMessage(`{"domain":"malicious.com"}`),
			Timestamp: time.Now(),
		},
		{
			Type:      "Page.loadEventFired",
			Data:      json.RawMessage(`{}`),
			Timestamp: time.Now(),
		},
		{
			Type:      "file_seen",
			Data:      json.RawMessage(`{"hash":"deadbeef","type":"pdf"}`),
			Timestamp: time.Now(),
		},
	}

	// Test filtering
	processableEvents := service.FilterProcessableEvents(events)
	require.Len(t, processableEvents, 2) // Only Network.requestWillBeSent and Network.responseReceived

	// Test statistics
	stats := service.GetFilterStats(events)
	assert.Equal(t, 6, stats.TotalEvents)
	assert.Equal(t, 2, stats.ProcessableEvents)
	assert.Equal(t, 4, stats.FilteredEvents)
	assert.InDelta(t, 66.67, stats.FilterRatio, 0.5) // 4/6 * 100 ≈ 66.67%

	// Test individual event processing decisions
	processingMap := service.ShouldProcessEvents(events)
	assert.True(t, processingMap[0])  // Network.requestWillBeSent
	assert.True(t, processingMap[1])  // Network.responseReceived
	assert.False(t, processingMap[2]) // Runtime.consoleAPICalled
	assert.False(t, processingMap[3]) // domain_seen (not processable by default)
	assert.False(t, processingMap[4]) // Page.loadEventFired
	assert.False(t, processingMap[5]) // file_seen (not processable by default)
}
