//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

import (
	"encoding/json"
	"time"
)

// JobResult represents persisted job execution details.
// JobID may be nil if the parent job has been reaped while preserving delivery history.
type JobResult struct {
	JobID     *string         `json:"job_id"     db:"job_id"`
	JobType   JobType         `json:"job_type"   db:"job_type"`
	Result    json.RawMessage `json:"result"     db:"result"`
	CreatedAt time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt time.Time       `json:"updated_at" db:"updated_at"`
}
