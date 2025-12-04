-- Add delivery_status to alerts with default 'pending'
ALTER TABLE alerts
    ADD COLUMN IF NOT EXISTS delivery_status TEXT NOT NULL DEFAULT 'pending'
        CHECK (delivery_status IN ('pending', 'muted', 'dispatched', 'failed'));

-- Backfill existing NULLs if any rows bypassed default during migration
UPDATE alerts
SET delivery_status = 'pending'
WHERE delivery_status IS NULL;
