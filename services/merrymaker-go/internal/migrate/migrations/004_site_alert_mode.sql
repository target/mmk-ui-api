-- Add alert_mode to sites with default 'active'
ALTER TABLE sites
    ADD COLUMN IF NOT EXISTS alert_mode TEXT NOT NULL DEFAULT 'active'
        CHECK (alert_mode IN ('active', 'muted'));

-- Backfill any existing NULLs just in case older rows bypassed defaults
UPDATE sites
SET alert_mode = 'active'
WHERE alert_mode IS NULL;
