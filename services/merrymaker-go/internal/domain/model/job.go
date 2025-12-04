// Package model defines the core data types and structures used throughout the merrymaker job system.
package model

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// JobType represents the type of job to be executed.
//
//nolint:recvcheck // UnmarshalText needs pointer receiver, Valid needs value receiver
type JobType string

// JobStatus represents the current status of a job.
type JobStatus string

const (
	// JobTypeBrowser represents a browser automation job type.
	JobTypeBrowser JobType = "browser"
	// JobTypeRules represents a rules processing job type.
	JobTypeRules JobType = "rules"
	// JobTypeAlert represents an HTTP alert dispatch job type.
	JobTypeAlert JobType = "alert"
	// JobTypeSecretRefresh represents a secret refresh job type.
	JobTypeSecretRefresh JobType = "secret_refresh"

	// JobStatusPending indicates a job is waiting to be processed.
	JobStatusPending JobStatus = "pending"
	// JobStatusRunning indicates a job is currently being processed.
	JobStatusRunning JobStatus = "running"
	// JobStatusCompleted indicates a job has finished successfully.
	JobStatusCompleted JobStatus = "completed"
	// JobStatusFailed indicates a job has failed to complete.
	JobStatusFailed JobStatus = "failed"
)

// UnmarshalText implements encoding.TextUnmarshaler for JobType to allow env parsing.
func (t *JobType) UnmarshalText(text []byte) error {
	v := strings.ToLower(strings.TrimSpace(string(text)))
	jt := JobType(v)
	if jt.Valid() {
		*t = jt
		return nil
	}
	return fmt.Errorf("invalid JobType: %q", v)
}

// ErrNoJobsAvailable is returned when no jobs are available for reservation.
var ErrNoJobsAvailable = errors.New("no jobs available")

// Valid returns true if the JobType is valid.
func (t JobType) Valid() bool {
	return t == JobTypeBrowser || t == JobTypeRules || t == JobTypeAlert || t == JobTypeSecretRefresh
}

// Valid returns true if the JobStatus is valid.
func (s JobStatus) Valid() bool {
	return s == JobStatusPending || s == JobStatusRunning || s == JobStatusCompleted ||
		s == JobStatusFailed
}

// Job represents a job in the system with all its metadata and status information.
type Job struct {
	ID             string          `json:"id"                         db:"id"`
	Type           JobType         `json:"type"                       db:"type"`
	Status         JobStatus       `json:"status"                     db:"status"`
	Priority       int             `json:"priority"                   db:"priority"`
	Payload        json.RawMessage `json:"payload"                    db:"payload"`
	Metadata       json.RawMessage `json:"metadata"                   db:"metadata"`
	SessionID      *string         `json:"session_id,omitempty"       db:"session_id"`
	SiteID         *string         `json:"site_id,omitempty"          db:"site_id"`
	SourceID       *string         `json:"source_id,omitempty"        db:"source_id"`
	IsTest         bool            `json:"is_test"                    db:"is_test"`
	ScheduledAt    time.Time       `json:"scheduled_at"               db:"scheduled_at"`
	StartedAt      *time.Time      `json:"started_at,omitempty"       db:"started_at"`
	CompletedAt    *time.Time      `json:"completed_at,omitempty"     db:"completed_at"`
	RetryCount     int             `json:"retry_count"                db:"retry_count"`
	MaxRetries     int             `json:"max_retries"                db:"max_retries"`
	LastError      *string         `json:"last_error,omitempty"       db:"last_error"`
	LeaseExpiresAt *time.Time      `json:"lease_expires_at,omitempty" db:"lease_expires_at"`
	CreatedAt      time.Time       `json:"created_at"                 db:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"                 db:"updated_at"`
}

// CreateJobRequest represents a request to create a new job.
type CreateJobRequest struct {
	Type        JobType         `json:"type"`
	Payload     json.RawMessage `json:"payload"`
	Metadata    json.RawMessage `json:"metadata,omitempty"`
	Priority    int             `json:"priority,omitempty"`
	SessionID   *string         `json:"session_id,omitempty"`
	SiteID      *string         `json:"site_id,omitempty"`
	SourceID    *string         `json:"source_id,omitempty"`
	IsTest      bool            `json:"is_test,omitempty"`
	ScheduledAt *time.Time      `json:"scheduled_at,omitempty"`
	MaxRetries  int             `json:"max_retries"`
}

// Validate validates the CreateJobRequest fields.
func (r *CreateJobRequest) Validate() error {
	if !r.Type.Valid() {
		return errors.New("invalid job type")
	}
	if len(r.Payload) == 0 {
		return errors.New("payload is required")
	}
	if r.Priority < 0 || r.Priority > 100 {
		return errors.New("priority must be between 0 and 100")
	}
	if r.MaxRetries < 0 {
		return errors.New("max retries must be >= 0")
	}
	return nil
}

// JobStats represents statistics about jobs in different states.
type JobStats struct {
	Pending   int `json:"pending"`
	Running   int `json:"running"`
	Completed int `json:"completed"`
	Failed    int `json:"failed"`
}

// JobStatusResponse represents the status information for a specific job.
type JobStatusResponse struct {
	Status      JobStatus  `json:"status"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	LastError   *string    `json:"last_error,omitempty"`
}

// BulkEventRequest represents a request to create multiple events in bulk.
type BulkEventRequest struct {
	SessionID   string     `json:"session_id"`
	SourceJobID *string    `json:"source_job_id,omitempty"`
	Events      []RawEvent `json:"events"`
}

// RawEvent represents a raw event with its data and metadata.
type RawEvent struct {
	Type       string          `json:"type"`
	Data       json.RawMessage `json:"data,omitempty"`
	StorageKey *string         `json:"storage_key,omitempty"`
	Timestamp  time.Time       `json:"timestamp"`
	Metadata   json.RawMessage `json:"metadata,omitempty"`
	Priority   *int            `json:"priority,omitempty"`
}

// PuppeteerEventBatch represents the event batch structure sent by the puppeteer worker.
type PuppeteerEventBatch struct {
	BatchID       string                 `json:"batchId"`
	SessionID     string                 `json:"sessionId"`
	Events        []PuppeteerEvent       `json:"events"`
	BatchMetadata PuppeteerBatchMetadata `json:"batchMetadata"`
	SequenceInfo  PuppeteerSequenceInfo  `json:"sequenceInfo"`
}

// PuppeteerEvent represents a single event from the puppeteer worker.
type PuppeteerEvent struct {
	ID       string                 `json:"id"`
	Method   string                 `json:"method"`
	Params   PuppeteerEventParams   `json:"params"`
	Metadata PuppeteerEventMetadata `json:"metadata"`
}

// PuppeteerEventParams contains the event parameters.
type PuppeteerEventParams struct {
	Timestamp   int64                `json:"timestamp"`
	SessionID   string               `json:"sessionId"`
	Attribution PuppeteerAttribution `json:"attribution"`
	Payload     map[string]any       `json:"payload"`
}

// PuppeteerAttribution contains attribution context.
type PuppeteerAttribution struct {
	URL       string `json:"url,omitempty"`
	UserAgent string `json:"userAgent,omitempty"`
}

// PuppeteerEventMetadata contains event metadata.
type PuppeteerEventMetadata struct {
	Category        string         `json:"category"`
	Tags            []string       `json:"tags"`
	ProcessingHints map[string]any `json:"processingHints"`
	SequenceNumber  int            `json:"sequenceNumber"`
}

// PuppeteerBatchMetadata contains batch metadata.
// Note: jobId is included to allow linking events to the source job in the DB.
type PuppeteerBatchMetadata struct {
	CreatedAt    int64                 `json:"createdAt"`
	EventCount   int                   `json:"eventCount"`
	TotalSize    int                   `json:"totalSize"`
	ChecksumInfo PuppeteerChecksumInfo `json:"checksumInfo"`
	RetryCount   int                   `json:"retryCount"`
	JobID        string                `json:"jobId,omitempty"`
}

// PuppeteerChecksumInfo contains checksum information.
type PuppeteerChecksumInfo struct {
	Algorithm string `json:"algorithm"`
	Value     string `json:"value"`
}

// PuppeteerSequenceInfo contains sequence information.
type PuppeteerSequenceInfo struct {
	SequenceNumber int  `json:"sequenceNumber"`
	IsFirstBatch   bool `json:"isFirstBatch"`
	IsLastBatch    bool `json:"isLastBatch"`
}

// Validate validates the BulkEventRequest fields and ensures it doesn't exceed the maximum batch size.
func (r *BulkEventRequest) Validate(maxBatch int) error {
	if r.SessionID == "" {
		return errors.New("session id is required")
	}
	if len(r.Events) == 0 {
		return errors.New("events is required")
	}
	if len(r.Events) > maxBatch {
		return errors.New("max batch size exceeded")
	}

	// Validate SourceJobID format if provided
	if r.SourceJobID != nil && *r.SourceJobID != "" {
		if _, err := uuid.Parse(*r.SourceJobID); err != nil {
			return errors.New("source job id must be a valid UUID")
		}
	}

	for i := range r.Events {
		if r.Events[i].Type == "" {
			return errors.New("event type is required")
		}
		if r.Events[i].Timestamp.IsZero() {
			return errors.New("timestamp is required")
		}
	}
	return nil
}

// Event represents an event in the system with all its metadata and status information.
type Event struct {
	ID            string          `json:"id"                      db:"id"`
	SessionID     string          `json:"session_id"              db:"session_id"`
	SourceJobID   *string         `json:"source_job_id,omitempty" db:"source_job_id"`
	EventType     string          `json:"event_type"              db:"event_type"`
	EventData     json.RawMessage `json:"event_data,omitempty"    db:"event_data"`
	Metadata      json.RawMessage `json:"metadata,omitempty"      db:"metadata"`
	StorageKey    *string         `json:"storage_key,omitempty"   db:"storage_key"`
	Priority      int             `json:"priority,omitempty"      db:"priority"`
	ShouldProcess bool            `json:"should_process"          db:"should_process"`
	Processed     bool            `json:"processed"               db:"processed"`
	CreatedAt     time.Time       `json:"created_at"              db:"created_at"`
}
