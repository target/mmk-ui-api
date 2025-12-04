package data

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

type eventCursorPayload struct {
	SortBy    string    `json:"sort_by"`
	SortDir   string    `json:"sort_dir"`
	EventType *string   `json:"event_type,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	ID        string    `json:"id"`
}

func encodeEventCursorPayload(cur eventCursorPayload) (string, error) {
	raw, err := json.Marshal(cur)
	if err != nil {
		return "", fmt.Errorf("marshal cursor: %w", err)
	}
	return base64.StdEncoding.EncodeToString(raw), nil
}

func decodeEventCursorPayload(token string) (eventCursorPayload, error) {
	raw, err := base64.StdEncoding.DecodeString(token)
	if err != nil {
		return eventCursorPayload{}, fmt.Errorf("decode cursor: %w", err)
	}

	var cur eventCursorPayload
	err = json.Unmarshal(raw, &cur)
	if err != nil {
		return eventCursorPayload{}, fmt.Errorf("unmarshal cursor: %w", err)
	}

	cur.SortDir = normalizeSortDir(cur.SortDir)
	cur.SortBy = normalizeSortBy(cur.SortBy)
	if cur.SortDir == "" || cur.SortBy == "" || cur.ID == "" || cur.CreatedAt.IsZero() {
		return eventCursorPayload{}, errors.New("invalid cursor payload")
	}
	if cur.SortBy == sortByEventType && (cur.EventType == nil || *cur.EventType == "") {
		return eventCursorPayload{}, errors.New("cursor missing event_type for sort")
	}

	return cur, nil
}

func newEventCursorFromEvent(ev *model.Event, sortBy, sortDir string) eventCursorPayload {
	payload := eventCursorPayload{
		SortBy:    normalizeSortBy(sortBy),
		SortDir:   normalizeSortDir(sortDir),
		CreatedAt: ev.CreatedAt,
		ID:        ev.ID,
	}
	if payload.SortBy == sortByEventType {
		payload.EventType = &ev.EventType
	}
	return payload
}

// EncodeEventCursorFromEvent builds a cursor token from the provided event using the supplied sort fields.
// Exposed for UI pagination so cursor-based navigation can be bootstrapped from the first page.
func EncodeEventCursorFromEvent(ev *model.Event, sortBy, sortDir string) (string, error) {
	if ev == nil {
		return "", errors.New("event is nil")
	}
	return encodeEventCursorPayload(newEventCursorFromEvent(ev, sortBy, sortDir))
}

func normalizeSortDir(dir string) string {
	switch strings.ToLower(dir) {
	case "", "asc":
		return sortDirAsc
	case "desc":
		return sortDirDesc
	default:
		return ""
	}
}

func normalizeSortBy(field string) string {
	switch strings.ToLower(field) {
	case "", "timestamp", defaultEventSortField:
		return defaultEventSortField
	case sortByEventType:
		return sortByEventType
	default:
		return ""
	}
}
