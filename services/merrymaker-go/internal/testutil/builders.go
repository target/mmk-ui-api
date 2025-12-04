// Package testutil provides testing utilities and helpers for the merrymaker job system.
package testutil

import (
	"encoding/json"
	"time"

	"github.com/target/mmk-ui-api/internal/domain/model"
)

// JobRequestBuilder provides a fluent interface for building CreateJobRequest objects for testing.
type JobRequestBuilder struct {
	req *model.CreateJobRequest
}

// NewJobRequest creates a new JobRequestBuilder with sensible defaults.
func NewJobRequest() *JobRequestBuilder {
	return &JobRequestBuilder{
		req: &model.CreateJobRequest{
			Type:       model.JobTypeBrowser,
			Priority:   50,
			Payload:    json.RawMessage(`{"url": "https://example.com"}`),
			MaxRetries: 3,
		},
	}
}

// WithType sets the job type.
func (b *JobRequestBuilder) WithType(jobType model.JobType) *JobRequestBuilder {
	b.req.Type = jobType
	return b
}

// WithPriority sets the job priority.
func (b *JobRequestBuilder) WithPriority(priority int) *JobRequestBuilder {
	b.req.Priority = priority
	return b
}

// WithPayload sets the job payload.
func (b *JobRequestBuilder) WithPayload(payload json.RawMessage) *JobRequestBuilder {
	b.req.Payload = payload
	return b
}

// WithPayloadString sets the job payload from a string.
func (b *JobRequestBuilder) WithPayloadString(payload string) *JobRequestBuilder {
	b.req.Payload = json.RawMessage(payload)
	return b
}

// WithMetadata sets the job metadata.
func (b *JobRequestBuilder) WithMetadata(metadata json.RawMessage) *JobRequestBuilder {
	b.req.Metadata = metadata
	return b
}

// WithMetadataString sets the job metadata from a string.
func (b *JobRequestBuilder) WithMetadataString(metadata string) *JobRequestBuilder {
	b.req.Metadata = json.RawMessage(metadata)
	return b
}

// WithSessionID sets the session ID.
func (b *JobRequestBuilder) WithSessionID(sessionID string) *JobRequestBuilder {
	b.req.SessionID = &sessionID
	return b
}

// WithScheduledAt sets the scheduled time.
func (b *JobRequestBuilder) WithScheduledAt(scheduledAt time.Time) *JobRequestBuilder {
	b.req.ScheduledAt = &scheduledAt
	return b
}

// WithMaxRetries sets the maximum number of retries.
func (b *JobRequestBuilder) WithMaxRetries(maxRetries int) *JobRequestBuilder {
	b.req.MaxRetries = maxRetries
	return b
}

// Build returns the constructed CreateJobRequest.
func (b *JobRequestBuilder) Build() *model.CreateJobRequest {
	return b.req
}

// TestScenarioBuilder provides a fluent interface for building test scenarios.
type TestScenarioBuilder struct {
	jobs []JobScenario
}

// JobScenario represents a job and the actions to perform on it.
type JobScenario struct {
	Request *model.CreateJobRequest
	Actions []JobAction
}

// JobAction represents an action to perform on a job.
type JobAction struct {
	Type   string // "reserve", "complete", "fail", "heartbeat"
	Params map[string]interface{}
}

// NewTestScenario creates a new TestScenarioBuilder.
func NewTestScenario() *TestScenarioBuilder {
	return &TestScenarioBuilder{
		jobs: make([]JobScenario, 0),
	}
}

// AddJob adds a job scenario to the test.
func (b *TestScenarioBuilder) AddJob(request *model.CreateJobRequest, actions ...JobAction) *TestScenarioBuilder {
	b.jobs = append(b.jobs, JobScenario{
		Request: request,
		Actions: actions,
	})
	return b
}

// AddPendingJob adds a job that stays pending.
func (b *TestScenarioBuilder) AddPendingJob(priority int) *TestScenarioBuilder {
	req := NewJobRequest().
		WithPriority(priority).
		WithPayloadString(`{"url": "https://pending.com"}`).
		Build()
	return b.AddJob(req)
}

// AddRunningJob adds a job that gets reserved and stays running.
func (b *TestScenarioBuilder) AddRunningJob(priority int) *TestScenarioBuilder {
	req := NewJobRequest().
		WithPriority(priority).
		WithPayloadString(`{"url": "https://running.com"}`).
		Build()
	return b.AddJob(req, ReserveAction())
}

// AddCompletedJob adds a job that gets reserved and completed.
func (b *TestScenarioBuilder) AddCompletedJob(priority int) *TestScenarioBuilder {
	req := NewJobRequest().
		WithPriority(priority).
		WithPayloadString(`{"url": "https://completed.com"}`).
		Build()
	return b.AddJob(req, ReserveAction(), CompleteAction())
}

// AddFailedJob adds a job that gets reserved and failed.
func (b *TestScenarioBuilder) AddFailedJob(priority, maxRetries int) *TestScenarioBuilder {
	req := NewJobRequest().
		WithPriority(priority).
		WithMaxRetries(maxRetries).
		WithPayloadString(`{"url": "https://failed.com"}`).
		Build()
	return b.AddJob(req, ReserveAction(), FailAction("test failure"))
}

// Build returns the constructed job scenarios.
func (b *TestScenarioBuilder) Build() []JobScenario {
	return b.jobs
}

// Action builders for common job actions

// ReserveAction creates a reserve action.
func ReserveAction() JobAction {
	return JobAction{Type: "reserve"}
}

// CompleteAction creates a complete action.
func CompleteAction() JobAction {
	return JobAction{Type: "complete"}
}

// FailAction creates a fail action with an error message.
func FailAction(errorMsg string) JobAction {
	return JobAction{
		Type:   "fail",
		Params: map[string]interface{}{"error": errorMsg},
	}
}

// HeartbeatAction creates a heartbeat action with lease seconds.
func HeartbeatAction(leaseSeconds int) JobAction {
	return JobAction{
		Type:   "heartbeat",
		Params: map[string]interface{}{"leaseSeconds": leaseSeconds},
	}
}

// Common test job request presets

// BrowserJobRequest creates a browser job request with default values.
func BrowserJobRequest() *model.CreateJobRequest {
	return NewJobRequest().
		WithType(model.JobTypeBrowser).
		WithPayloadString(`{"url": "https://example.com", "selector": "body"}`).
		Build()
}

// RulesJobRequest creates a rules job request with default values.
func RulesJobRequest() *model.CreateJobRequest {
	return NewJobRequest().
		WithType(model.JobTypeRules).
		WithPayloadString(`{"rules": ["rule1", "rule2"]}`).
		Build()
}

// HighPriorityJobRequest creates a high priority job request.
func HighPriorityJobRequest() *model.CreateJobRequest {
	return NewJobRequest().
		WithPriority(100).
		WithPayloadString(`{"urgent": true}`).
		Build()
}

// LowPriorityJobRequest creates a low priority job request.
func LowPriorityJobRequest() *model.CreateJobRequest {
	return NewJobRequest().
		WithPriority(10).
		WithPayloadString(`{"background": true}`).
		Build()
}

// ScheduledJobRequest creates a job request scheduled for the future.
func ScheduledJobRequest(scheduledAt time.Time) *model.CreateJobRequest {
	return NewJobRequest().
		WithScheduledAt(scheduledAt).
		WithPayloadString(`{"scheduled": true}`).
		Build()
}

// RetryableJobRequest creates a job request with custom retry settings.
func RetryableJobRequest(maxRetries int) *model.CreateJobRequest {
	return NewJobRequest().
		WithMaxRetries(maxRetries).
		WithPayloadString(`{"retryable": true}`).
		Build()
}
