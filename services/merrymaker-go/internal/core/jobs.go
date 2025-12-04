// Package core provides the business logic and service layer for the merrymaker job system.
package core

import (
	"github.com/target/mmk-ui-api/internal/domain/model"
)

// JobType represents the type of job to be executed (re-exported from types package).
// This is re-exported here for use in HTTP handlers to avoid direct coupling to the types package.
type JobType = model.JobType

// CreateJobRequest represents a request to create a new job (re-exported from types package).
// This is re-exported here for use in HTTP handlers to avoid direct coupling to the types package.
type CreateJobRequest = model.CreateJobRequest
