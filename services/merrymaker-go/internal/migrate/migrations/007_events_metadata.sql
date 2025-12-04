-- Add metadata column to events for storing request attribution and related context

ALTER TABLE events
ADD COLUMN IF NOT EXISTS metadata JSONB NOT NULL DEFAULT '{}'::jsonb;

-- Backfill existing rows to ensure non-null values
UPDATE events SET metadata = '{}'::jsonb WHERE metadata IS NULL;
