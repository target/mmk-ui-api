//revive:disable-next-line:var-naming // legacy package name widely used across the project
package model

// JobListBySourceOptions groups parameters for listing jobs by source.
type JobListBySourceOptions struct {
	SourceID string
	Limit    int
	Offset   int
}

// JobListBySiteOptions groups parameters for listing jobs by site with optional filters.
type JobListBySiteOptions struct {
	SiteID *string // Optional filter by site_id
	Status *string // Optional filter by status (pending, running, completed, failed)
	Type   *string // Optional filter by type (browser, rules, alert)
	Limit  int     // Pagination limit
	Offset int     // Pagination offset
}

// JobListOptions groups parameters for listing all jobs with optional filters (admin view).
type JobListOptions struct {
	Status    *JobStatus // Optional filter by status (pending, running, completed, failed)
	Type      *JobType   // Optional filter by type (browser, rules, alert)
	SiteID    *string    // Optional filter by site_id
	IsTest    *bool      // Optional filter by is_test flag
	SortBy    string     // Sort field: "created_at", "status", "type" (default: "created_at")
	SortOrder string     // Sort order: "asc", "desc" (default: "desc")
	Limit     int        // Pagination limit
	Offset    int        // Pagination offset
}

// JobWithEventCount represents a job with its associated event count for UI display.
type JobWithEventCount struct {
	Job
	EventCount int    `json:"event_count"         db:"event_count"`
	SiteName   string `json:"site_name,omitempty" db:"site_name"`
}
