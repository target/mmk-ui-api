package service

import (
	"strings"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// EventFilterService determines which events should be processed by the rules engine.
type EventFilterService struct {
	processableTypes map[string]bool // original-case keys (immutable after construction)
	normalized       map[string]bool // lowercase-normalized view for case-insensitive lookups
}

// NewEventFilterService creates a new event filter service.
// Optionally pass default processable event types; if none provided, the built-in defaults are used.
func NewEventFilterService(defaults ...string) *EventFilterService {
	// Built-in defaults: CDP Network events used for domain extraction and analysis.
	const defaultType1, defaultType2 = "Network.requestWillBeSent", "Network.responseReceived"

	proces := make(map[string]bool, len(defaults)+2)
	for _, t := range defaults {
		proces[t] = true
	}

	if len(proces) == 0 {
		proces[defaultType1] = true
		proces[defaultType2] = true
	}

	norm := make(map[string]bool, len(proces))
	for k := range proces {
		norm[strings.ToLower(k)] = true
	}

	return &EventFilterService{
		processableTypes: proces,
		normalized:       norm,
	}
}

// ShouldProcessEvent determines if an event should be processed by the rules engine.
func (s *EventFilterService) ShouldProcessEvent(eventType string) bool {
	return s.normalized[strings.ToLower(strings.TrimSpace(eventType))]
}

// ShouldProcessEvents determines which events in a batch should be processed.
// Returns a map of event index to boolean indicating if that event should be processed.
func (s *EventFilterService) ShouldProcessEvents(events []model.RawEvent) map[int]bool {
	result := make(map[int]bool, len(events))

	for i, event := range events {
		result[i] = s.ShouldProcessEvent(event.Type)
	}

	return result
}

// FilterProcessableEvents returns only the events that should be processed by the rules engine.
func (s *EventFilterService) FilterProcessableEvents(events []model.RawEvent) []model.RawEvent {
	var processableEvents []model.RawEvent

	for _, event := range events {
		if s.ShouldProcessEvent(event.Type) {
			processableEvents = append(processableEvents, event)
		}
	}

	return processableEvents
}

// EventFilterStats represents statistics about event filtering.
type EventFilterStats struct {
	TotalEvents       int     `json:"total_events"`
	ProcessableEvents int     `json:"processable_events"`
	FilteredEvents    int     `json:"filtered_events"`
	FilterRatio       float64 `json:"filter_ratio"` // Percentage of events filtered out
}

// GetFilterStats calculates filtering statistics for a batch of events.
func (s *EventFilterService) GetFilterStats(events []model.RawEvent) EventFilterStats {
	total := len(events)
	processable := 0

	for _, event := range events {
		if s.ShouldProcessEvent(event.Type) {
			processable++
		}
	}

	filtered := total - processable
	filterRatio := 0.0
	if total > 0 {
		filterRatio = float64(filtered) / float64(total) * 100.0
	}

	return EventFilterStats{
		TotalEvents:       total,
		ProcessableEvents: processable,
		FilteredEvents:    filtered,
		FilterRatio:       filterRatio,
	}
}
