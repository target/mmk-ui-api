-- Performance indexes for slow/expensive queries observed via pg_stat_statements.
-- Note: Do not use CREATE INDEX CONCURRENTLY; migrations run inside a transaction in tests.

-- Speeds up job event lookups by source_job_id (used in ListByJob/ListWithFilters).
CREATE INDEX IF NOT EXISTS idx_events_source_job_created ON events (source_job_id, created_at, id);
-- Variant with event_type first helps when filtering/sorting by event_type for a job.
CREATE INDEX IF NOT EXISTS idx_events_source_job_event_type_created ON events (source_job_id, event_type, created_at, id);

-- Helps reaper DELETE of completed/failed jobs ordered by completion/update time.
CREATE INDEX IF NOT EXISTS idx_jobs_completed_failed_coalesce_ts
  ON jobs (status, COALESCE(completed_at, updated_at))
  WHERE status IN ('completed', 'failed');
