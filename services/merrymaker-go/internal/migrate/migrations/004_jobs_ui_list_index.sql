-- Composite index to optimize UI jobs list queries
-- Test/CI-safe index creation (runs inside transaction)
CREATE INDEX IF NOT EXISTS idx_jobs_ui_list
    ON jobs (site_id, status, type, is_test, created_at, id);


