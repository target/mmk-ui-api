-- Precomputed job metadata to avoid expensive event scans on UI/job views.

CREATE TABLE IF NOT EXISTS job_meta (
    job_id UUID PRIMARY KEY REFERENCES jobs(id) ON DELETE CASCADE,
    event_count INTEGER NOT NULL DEFAULT 0,
    last_status TEXT,
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Backfill existing rows so historical jobs have precomputed event counts.
INSERT INTO job_meta (job_id, event_count, last_status)
SELECT j.id,
       COALESCE(ec.event_count, 0),
       j.status
FROM jobs j
LEFT JOIN (
    SELECT source_job_id, COUNT(*) AS event_count
    FROM events
    GROUP BY source_job_id
) ec ON ec.source_job_id = j.id
ON CONFLICT (job_id) DO UPDATE
SET event_count = EXCLUDED.event_count,
    last_status = EXCLUDED.last_status,
    updated_at = now();

CREATE INDEX IF NOT EXISTS idx_job_meta_updated_at ON job_meta (updated_at);
