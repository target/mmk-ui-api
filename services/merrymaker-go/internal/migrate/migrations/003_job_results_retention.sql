-- Reshape job_results so we can keep delivery history after reaping jobs.
ALTER TABLE job_results
    DROP CONSTRAINT IF EXISTS job_results_job_id_fkey;

ALTER TABLE job_results
    DROP CONSTRAINT IF EXISTS job_results_pkey;

ALTER TABLE job_results
    ADD COLUMN IF NOT EXISTS id UUID;

UPDATE job_results
SET id = gen_random_uuid()
WHERE id IS NULL;

ALTER TABLE job_results
    ALTER COLUMN id SET DEFAULT gen_random_uuid();

ALTER TABLE job_results
    ALTER COLUMN id SET NOT NULL;

ALTER TABLE job_results
    ALTER COLUMN job_id DROP NOT NULL;

ALTER TABLE job_results
    ADD CONSTRAINT job_results_pkey PRIMARY KEY (id);

ALTER TABLE job_results
    ADD CONSTRAINT job_results_job_id_fkey
        FOREIGN KEY (job_id) REFERENCES jobs(id) ON DELETE SET NULL;

CREATE UNIQUE INDEX IF NOT EXISTS job_results_job_id_key
    ON job_results (job_id);

-- Improve lookup performance when listing results by alert id.
CREATE INDEX IF NOT EXISTS job_results_alert_id_idx
    ON job_results ((result ->> 'alert_id'));

-- Support batched retention cleanup queries.
CREATE INDEX IF NOT EXISTS job_results_job_type_updated_at_idx
    ON job_results (job_type, updated_at, job_id);
