//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

// EventListByJobOptions groups parameters for listing/counting events by job with optional filters.
type EventListByJobOptions struct {
	JobID  string
	Limit  int
	Offset int
	// CursorAfter and CursorBefore enable keyset pagination. When provided, they take precedence over Offset.
	CursorAfter  *string
	CursorBefore *string
	// Optional filters (when nil/empty, no filter is applied)
	EventType   *string // Optional filter by exact event_type (e.g., "Network.requestWillBeSent")
	Category    *string // Optional filter by event category (network, console, security, page, action, error, other)
	SearchQuery *string // Optional text search in event_data JSON
	SortBy      *string // Optional sort field (timestamp, event_type)
	SortDir     *string // Optional sort direction (asc, desc)
}

// EventListPage contains a page of events with pagination cursors.
type EventListPage struct {
	Events     []*Event
	NextCursor *string
	PrevCursor *string
}

// EventListOptions is an alias for EventListByJobOptions for backward compatibility.
//
// Deprecated: Use EventListByJobOptions instead.
type EventListOptions = EventListByJobOptions
