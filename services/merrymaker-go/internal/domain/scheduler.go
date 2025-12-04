// Package domain contains domain-specific business logic and entities for the merrymaker job system.
package domain

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// ScheduledTask represents a scheduled task that can be queued as jobs at regular intervals.
type ScheduledTask struct {
	ID       string          `json:"id"`
	TaskName string          `json:"task_name"`
	Payload  json.RawMessage `json:"payload"`
	// Interval is the scheduling cadence.
	// Note: encoding/json marshals time.Duration as a number of nanoseconds.
	// If this type becomes part of external config or API, consider a string-encoded duration (e.g., "30s", "1m").
	Interval     time.Duration `json:"interval"`
	LastQueuedAt *time.Time    `json:"last_queued_at,omitempty"`
	UpdatedAt    time.Time     `json:"updated_at"`
	// OverrunPolicy overrides the scheduler strategy when set; otherwise global defaults are used.
	OverrunPolicy *OverrunPolicy `json:"overrun_policy,omitempty"`
	// OverrunStates defines which job states should block new enqueue attempts.
	OverrunStates *OverrunStateMask `json:"overrun_states,omitempty"`
	// ActiveFireKey tracks the currently outstanding fire key (if any) for the task.
	ActiveFireKey *string `json:"active_fire_key,omitempty"`
}

// OverrunPolicy defines how to handle scheduling when a previous job is still running.
type OverrunPolicy string

const (
	// OverrunPolicySkip skips scheduling if a running job with unexpired lease exists.
	// Expired leases should not block scheduling (addresses downstream/network failures).
	OverrunPolicySkip OverrunPolicy = "skip"

	// OverrunPolicyQueue always enqueues a new job regardless of running jobs.
	OverrunPolicyQueue OverrunPolicy = "queue"

	// OverrunPolicyReschedule updates last_queued_at but does not enqueue a job.
	OverrunPolicyReschedule OverrunPolicy = "reschedule"
)

// OverrunStateMask controls which job states block new enqueue attempts when using OverrunPolicySkip.
// Bitmask values allow callers to toggle multiple states at once.
type OverrunStateMask uint8

const (
	// OverrunStateRunning blocks when an in-progress job with an active lease exists.
	OverrunStateRunning OverrunStateMask = 1 << iota
	// OverrunStatePending blocks when a pending job exists (covers freshly enqueued jobs).
	OverrunStatePending
	// OverrunStateRetrying blocks when a pending job exists with retry_count > 0.
	OverrunStateRetrying
)

// OverrunStatesDefault is the historical behavior of blocking only on running jobs.
const OverrunStatesDefault = OverrunStateRunning

// Has reports whether the mask includes the provided flag.
func (m *OverrunStateMask) Has(flag OverrunStateMask) bool {
	if m == nil {
		return false
	}
	return (*m)&flag != 0
}

// String returns a stable, comma-separated representation of the mask.
func (m *OverrunStateMask) String() string {
	if m == nil {
		return ""
	}
	mask := *m
	if mask == 0 {
		return ""
	}

	var parts []string
	for _, entry := range []struct {
		name string
		flag OverrunStateMask
	}{
		{"running", OverrunStateRunning},
		{"pending", OverrunStatePending},
		{"retrying", OverrunStateRetrying},
	} {
		if mask&entry.flag != 0 {
			parts = append(parts, entry.name)
		}
	}
	return strings.Join(parts, ",")
}

// ParseOverrunStateMask parses a comma-separated list of state names into a mask.
func ParseOverrunStateMask(v string) (OverrunStateMask, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, nil
	}
	var mask OverrunStateMask
	for _, part := range strings.Split(v, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		flag, err := parseOverrunStateName(name)
		if err != nil {
			return 0, err
		}
		mask |= flag
	}
	return mask, nil
}

// MarshalText implements encoding.TextMarshaler.
func (m *OverrunStateMask) MarshalText() ([]byte, error) {
	return []byte(m.String()), nil
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *OverrunStateMask) UnmarshalText(text []byte) error {
	mask, err := ParseOverrunStateMask(string(text))
	if err != nil {
		return err
	}
	*m = mask
	return nil
}

func parseOverrunStateName(name string) (OverrunStateMask, error) {
	switch name {
	case "running":
		return OverrunStateRunning, nil
	case "pending":
		return OverrunStatePending, nil
	case "retrying":
		return OverrunStateRetrying, nil
	default:
		return 0, fmt.Errorf("invalid overrun state: %q", name)
	}
}

// UnmarshalText implements encoding.TextUnmarshaler to parse OverrunPolicy from env or text.
func (p *OverrunPolicy) UnmarshalText(text []byte) error {
	v := strings.ToLower(strings.TrimSpace(string(text)))
	switch OverrunPolicy(v) {
	case OverrunPolicySkip, OverrunPolicyQueue, OverrunPolicyReschedule:
		*p = OverrunPolicy(v)
		return nil
	default:
		return fmt.Errorf("invalid OverrunPolicy: %q", v)
	}
}

// StrategyOptions holds configuration for scheduling strategy.
type StrategyOptions struct {
	Overrun       OverrunPolicy    `json:"overrun"`
	OverrunStates OverrunStateMask `json:"overrun_states"`
}

// FindDueParams holds inputs for transactional FindDue.
type FindDueParams struct {
	Now   time.Time
	Limit int
}

// MarkQueuedParams holds inputs for transactional MarkQueued.
type MarkQueuedParams struct {
	ID                 string
	Now                time.Time
	ActiveFireKey      *string
	ActiveFireKeySetAt *time.Time
}

// UpdateActiveFireKeyParams updates the outstanding fire key for a scheduled task.
// Provide FireKey=nil to clear the active key.
type UpdateActiveFireKeyParams struct {
	ID      string
	FireKey *string
	SetAt   time.Time
}

// UpsertTaskParams holds parameters for admin upsert-by-name in scheduled_jobs.
// Keeping params in a struct maintains the ≤3 parameter guideline.
type UpsertTaskParams struct {
	TaskName string
	Payload  json.RawMessage
	Interval time.Duration
	// Optional overrides. When nil the scheduler applies global defaults.
	OverrunPolicy *OverrunPolicy
	OverrunStates *OverrunStateMask
}
