package notify

import (
	"context"
	"time"
)

// Severity constants recognised by downstream sinks.
const (
	SeverityCritical = "critical"
)

// JobFailurePayload captures the canonical data we emit for job failure notifications.
type JobFailurePayload struct {
	JobID      string
	JobType    string
	SiteID     string
	SiteName   string
	Scope      string
	IsTest     bool
	Error      string
	ErrorClass string
	Severity   string
	OccurredAt time.Time
	Metadata   map[string]string
}

// Sink describes a destination capable of consuming job failure notifications.
type Sink interface {
	SendJobFailure(ctx context.Context, payload JobFailurePayload) error
}

// SinkFunc adapts a function to the Sink interface (useful for tests).
type SinkFunc func(ctx context.Context, payload JobFailurePayload) error

// SendJobFailure implements the Sink interface.
func (f SinkFunc) SendJobFailure(ctx context.Context, payload JobFailurePayload) error {
	if f == nil {
		return nil
	}
	return f(ctx, payload)
}
