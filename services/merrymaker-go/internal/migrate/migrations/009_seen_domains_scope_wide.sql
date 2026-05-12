-- Make seen_domains scope-wide across sites.
-- Deduplicate existing rows and swap the unique index to (domain, scope).

-- Consolidate duplicates by (domain, scope). Keep the earliest row's site_id, first_seen_at, and created_at; sum hit_count; and preserve the most recent last_seen_at.
CREATE TEMP TABLE tmp_seen_domains_ranked AS
SELECT
    id,
    domain,
    scope,
    site_id,
    first_seen_at,
    last_seen_at,
    hit_count,
    created_at,
    row_number() OVER (PARTITION BY domain, scope ORDER BY first_seen_at ASC, created_at ASC, id ASC) AS rn,
    min(first_seen_at) OVER (PARTITION BY domain, scope) AS min_first_seen,
    max(last_seen_at) OVER (PARTITION BY domain, scope) AS max_last_seen,
    sum(hit_count) OVER (PARTITION BY domain, scope) AS sum_hit_count,
    min(created_at) OVER (PARTITION BY domain, scope) AS min_created_at
FROM seen_domains;

-- Update the keeper rows with aggregated values.
UPDATE seen_domains sd
SET first_seen_at = r.min_first_seen,
    last_seen_at = GREATEST(sd.last_seen_at, r.max_last_seen),
    hit_count = r.sum_hit_count,
    created_at = LEAST(sd.created_at, r.min_created_at)
FROM tmp_seen_domains_ranked r
WHERE sd.id = r.id AND r.rn = 1;

-- Remove duplicate rows, keeping only the ranked keeper.
DELETE FROM seen_domains sd
USING tmp_seen_domains_ranked r
WHERE sd.id = r.id AND r.rn > 1;

DROP TABLE tmp_seen_domains_ranked;

-- Replace the uniqueness constraint to be scope-wide.
DROP INDEX IF EXISTS idx_seen_domains_site_domain_scope;
CREATE UNIQUE INDEX IF NOT EXISTS idx_seen_domains_domain_scope ON seen_domains (domain, scope);
