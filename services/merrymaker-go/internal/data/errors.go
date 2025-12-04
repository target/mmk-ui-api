package data

import "errors"

// Shared sentinel errors for data-layer repositories.
var (
	// IOC repository sentinels.
	ErrIOCNotFound      = errors.New("IOC not found")
	ErrIOCAlreadyExists = errors.New("IOC already exists")

	// Job result repository sentinels.
	ErrJobResultsNotConfigured = errors.New("job results repository not configured")
	ErrJobResultsNotFound      = errors.New("job results not found")
	ErrJobIDRequired           = errors.New("job_id is required")
	ErrAlertIDRequired         = errors.New("alert_id is required")
)
