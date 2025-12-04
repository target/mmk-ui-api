-- Migration: add per-task overrun configuration and outstanding fire key tracking.
ALTER TABLE scheduled_jobs
    ADD COLUMN IF NOT EXISTS overrun_policy TEXT,
    ADD COLUMN IF NOT EXISTS overrun_state_mask SMALLINT,
    ADD COLUMN IF NOT EXISTS active_fire_key TEXT,
    ADD COLUMN IF NOT EXISTS active_fire_key_set_at TIMESTAMPTZ;

ALTER TABLE scheduled_jobs
    ADD CONSTRAINT scheduled_jobs_overrun_policy_check
    CHECK (
        overrun_policy IS NULL OR
        overrun_policy IN ('skip', 'queue', 'reschedule')
    );

CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_active_fire_key
    ON scheduled_jobs (task_name)
    WHERE active_fire_key IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_scheduled_jobs_task_fire_key
    ON scheduled_jobs (task_name, active_fire_key)
    WHERE active_fire_key IS NOT NULL;
