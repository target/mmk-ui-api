package service

import (
	"maps"
	"strings"
	"sync"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// EventFilterService determines which events should be processed by the rules engine.
type EventFilterService struct {
	mu sync.RWMutex
	// ProcessableEventTypes defines which event types should be processed by rules engine
	ProcessableEventTypes map[string]bool
	// normalized holds a lowercase-normalized view for efficient case-insensitive lookups
	normalized map[string]bool
}

// NewEventFilterService creates a new event filter service with default processable event model.
func NewEventFilterService() *EventFilterService {
	// Keep original-case keys for compatibility while also building a normalized map
	orig := map[string]bool{
		// CDP Network events used for domain extraction and analysis
		"Network.requestWillBeSent": true,
		"Network.responseReceived":  true,
	}
	s := &EventFilterService{ProcessableEventTypes: orig, normalized: make(map[string]bool, len(orig))}
	for k, v := range orig {
		s.normalized[strings.ToLower(k)] = v
	}
	return s
}

// ShouldProcessEvent determines if an event should be processed by the rules engine.
func (s *EventFilterService) ShouldProcessEvent(eventType string) bool {
	et := strings.ToLower(strings.TrimSpace(eventType))
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.normalized[et]
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

// AddProcessableEventType adds a new event type to the processable list.
func (s *EventFilterService) AddProcessableEventType(eventType string) {
	et := strings.TrimSpace(eventType)
	if et == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ProcessableEventTypes[et] = true
	s.normalized[strings.ToLower(et)] = true
}

// RemoveProcessableEventType removes an event type from the processable list.
func (s *EventFilterService) RemoveProcessableEventType(eventType string) {
	et := strings.TrimSpace(eventType)
	if et == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.ProcessableEventTypes, et)
	delete(s.normalized, strings.ToLower(et))
}

// SetProcessableEventType sets whether an event type should be processed.
func (s *EventFilterService) SetProcessableEventType(eventType string, shouldProcess bool) {
	et := strings.TrimSpace(eventType)
	if et == "" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ProcessableEventTypes[et] = shouldProcess
	s.normalized[strings.ToLower(et)] = shouldProcess
}

// GetProcessableEventTypes returns a copy of the current processable event model.
func (s *EventFilterService) GetProcessableEventTypes() map[string]bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make(map[string]bool, len(s.ProcessableEventTypes))
	maps.Copy(result, s.ProcessableEventTypes)
	return result
}

// GetProcessableEventTypesList returns a list of event types that should be processed.
func (s *EventFilterService) GetProcessableEventTypesList() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var eventTypes []string
	for eventType, shouldProcess := range s.ProcessableEventTypes {
		if shouldProcess {
			eventTypes = append(eventTypes, eventType)
		}
	}
	return eventTypes
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
