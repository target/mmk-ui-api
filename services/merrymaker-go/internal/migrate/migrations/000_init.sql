-- Consolidated database initialization migration
-- This migration creates the complete database schema from scratch

-- ============================================================================
-- EXTENSIONS
-- ============================================================================

-- Enable pgcrypto for UUID generation
DO $$
BEGIN
  CREATE EXTENSION IF NOT EXISTS pgcrypto;
EXCEPTION WHEN duplicate_object OR unique_violation THEN
  NULL;
END
$$;

-- Enable pg_trgm for trigram search support (best-effort; ignore if lacking privileges)
DO $$
BEGIN
  CREATE EXTENSION IF NOT EXISTS pg_trgm;
EXCEPTION WHEN insufficient_privilege OR duplicate_object OR unique_violation THEN
  NULL;
END
$$;

-- ============================================================================
-- CUSTOM TYPES
-- ============================================================================

DO $$ BEGIN
    CREATE TYPE ioc_type AS ENUM ('fqdn', 'ip');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- ============================================================================
-- FUNCTIONS
-- ============================================================================

CREATE OR REPLACE FUNCTION set_updated_at() RETURNS TRIGGER AS $$
BEGIN
  NEW.updated_at = NOW();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ============================================================================
-- TABLES
-- ============================================================================

-- Jobs table
CREATE TABLE IF NOT EXISTS jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type TEXT NOT NULL CHECK (type IN ('browser', 'rules', 'alert', 'secret_refresh')),
    status TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    priority INTEGER NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 100),
    payload JSONB NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    session_id UUID,
    scheduled_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    retry_count INTEGER NOT NULL DEFAULT 0 CHECK (retry_count >= 0),
    max_retries INTEGER NOT NULL DEFAULT 3 CHECK (max_retries >= 0),
    last_error TEXT,
    lease_expires_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    site_id UUID,
    source_id UUID,
    is_test BOOLEAN NOT NULL DEFAULT FALSE
);

-- Events table
CREATE TABLE IF NOT EXISTS events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    session_id UUID NOT NULL,
    source_job_id UUID,
    event_type VARCHAR(100) NOT NULL,
    event_data JSONB,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    storage_key VARCHAR(500),
    priority INTEGER NOT NULL DEFAULT 0 CHECK (priority BETWEEN 0 AND 100),
    should_process BOOLEAN NOT NULL DEFAULT FALSE,
    processed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Scheduled Jobs table
CREATE TABLE IF NOT EXISTS scheduled_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    task_name TEXT NOT NULL UNIQUE,
    payload JSONB,
    scheduled_interval INTERVAL NOT NULL,
    last_queued_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Sources table
CREATE TABLE IF NOT EXISTS sources (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL CHECK (btrim(name) <> ''),
    value TEXT NOT NULL,
    test BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Secrets table
CREATE TABLE IF NOT EXISTS secrets (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL UNIQUE CHECK (btrim(name) <> ''),
    value TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    provider_script_path TEXT,
    env_config JSONB DEFAULT '{}'::jsonb,
    refresh_interval INTERVAL,
    last_refreshed_at TIMESTAMPTZ,
    last_refresh_status VARCHAR(20) CHECK (last_refresh_status IN ('success', 'failed', 'pending')),
    last_refresh_error TEXT,
    refresh_enabled BOOLEAN NOT NULL DEFAULT FALSE,
    CONSTRAINT secrets_refresh_config_check CHECK (
        (refresh_enabled = FALSE) OR
        (refresh_enabled = TRUE AND provider_script_path IS NOT NULL AND btrim(provider_script_path) <> '' AND refresh_interval IS NOT NULL)
    )
);

-- Source Secrets association table
CREATE TABLE IF NOT EXISTS source_secrets (
    source_id UUID NOT NULL,
    secret_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (source_id, secret_id)
);

-- HTTP Alert Sinks table
CREATE TABLE IF NOT EXISTS http_alert_sinks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(512) NOT NULL CHECK (char_length(btrim(name)) >= 3 AND char_length(btrim(name)) <= 512),
    uri VARCHAR(1024) NOT NULL CHECK (btrim(uri) <> ''),
    method VARCHAR(10) NOT NULL CHECK (upper(method) IN ('GET', 'POST', 'PUT', 'PATCH', 'DELETE')),
    body TEXT,
    query_params TEXT,
    headers TEXT,
    ok_status INTEGER NOT NULL DEFAULT 200 CHECK (ok_status BETWEEN 100 AND 599),
    retry INTEGER NOT NULL DEFAULT 3 CHECK (retry >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- HTTP Alert Sink Secrets association table
CREATE TABLE IF NOT EXISTS http_alert_sink_secrets (
    http_alert_sink_id UUID NOT NULL,
    secret_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (http_alert_sink_id, secret_id)
);

-- Sites table
CREATE TABLE IF NOT EXISTS sites (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL CHECK (btrim(name) <> ''),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    alert_mode TEXT NOT NULL DEFAULT 'active' CHECK (alert_mode IN ('active', 'muted')),
    scope TEXT,
    http_alert_sink_id UUID,
    last_enabled TIMESTAMPTZ,
    last_run TIMESTAMPTZ,
    run_every_minutes INTEGER NOT NULL CHECK (run_every_minutes > 0),
    source_id UUID NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Alerts table
CREATE TABLE IF NOT EXISTS alerts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL,
    rule_id UUID,
    rule_type VARCHAR(50) NOT NULL CHECK (rule_type IN ('unknown_domain', 'ioc_domain', 'yara_rule', 'custom')),
    severity VARCHAR(20) NOT NULL CHECK (severity IN ('critical', 'high', 'medium', 'low', 'info')),
    title VARCHAR(255) NOT NULL CHECK (char_length(btrim(title)) >= 1),
    description TEXT NOT NULL CHECK (char_length(btrim(description)) >= 1),
    event_context JSONB NOT NULL DEFAULT '{}'::jsonb,
    metadata JSONB DEFAULT '{}'::jsonb,
    delivery_status TEXT NOT NULL DEFAULT 'pending' CHECK (delivery_status IN ('pending', 'muted', 'dispatched', 'failed')),
    fired_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_by VARCHAR(255)
);

-- Rules table
CREATE TABLE IF NOT EXISTS rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL,
    rule_type VARCHAR(50) NOT NULL CHECK (rule_type IN ('unknown_domain', 'ioc_domain', 'yara_rule', 'custom')),
    config JSONB NOT NULL DEFAULT '{}'::jsonb,
    enabled BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER NOT NULL DEFAULT 100 CHECK (priority BETWEEN 1 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Seen Domains table
CREATE TABLE IF NOT EXISTS seen_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL,
    domain VARCHAR(255) NOT NULL CHECK (char_length(btrim(domain)) >= 1),
    scope VARCHAR(100) NOT NULL DEFAULT 'default',
    first_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    hit_count INTEGER NOT NULL DEFAULT 1 CHECK (hit_count >= 1),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Processed Files table
CREATE TABLE IF NOT EXISTS processed_files (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id UUID NOT NULL,
    file_hash VARCHAR(64) NOT NULL CHECK (char_length(file_hash) = 64),
    storage_key VARCHAR(500) NOT NULL,
    scope VARCHAR(100) NOT NULL DEFAULT 'default',
    yara_results JSONB DEFAULT '{}'::jsonb,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Domain Allowlists table
CREATE TABLE IF NOT EXISTS domain_allowlists (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    scope VARCHAR(100) DEFAULT 'default',
    pattern VARCHAR(255) NOT NULL CHECK (char_length(btrim(pattern)) >= 1),
    pattern_type VARCHAR(20) NOT NULL DEFAULT 'exact' CHECK (pattern_type IN ('exact', 'wildcard', 'glob', 'etld_plus_one')),
    description TEXT,
    enabled BOOLEAN NOT NULL DEFAULT true,
    priority INTEGER NOT NULL DEFAULT 100 CHECK (priority BETWEEN 1 AND 1000),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- IOC Domains table
CREATE TABLE IF NOT EXISTS ioc_domains (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    domain VARCHAR(255) NOT NULL CHECK (char_length(btrim(domain)) >= 1),
    source VARCHAR(100) NOT NULL CHECK (char_length(btrim(source)) >= 1),
    severity VARCHAR(20) NOT NULL DEFAULT 'medium' CHECK (severity IN ('critical', 'high', 'medium', 'low')),
    description TEXT,
    metadata JSONB DEFAULT '{}'::jsonb,
    added_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at TIMESTAMPTZ,
    UNIQUE (domain)
);

-- IOCs table
CREATE TABLE IF NOT EXISTS iocs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type ioc_type NOT NULL,
    value TEXT NOT NULL,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    description TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ============================================================================
-- FOREIGN KEY CONSTRAINTS
-- ============================================================================

ALTER TABLE events ADD CONSTRAINT events_source_job_id_fkey
    FOREIGN KEY (source_job_id) REFERENCES jobs(id) ON DELETE SET NULL;

ALTER TABLE source_secrets ADD CONSTRAINT source_secrets_source_id_fkey
    FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE CASCADE;
ALTER TABLE source_secrets ADD CONSTRAINT source_secrets_secret_id_fkey
    FOREIGN KEY (secret_id) REFERENCES secrets(id) ON DELETE RESTRICT;

ALTER TABLE http_alert_sink_secrets ADD CONSTRAINT http_alert_sink_secrets_http_alert_sink_id_fkey
    FOREIGN KEY (http_alert_sink_id) REFERENCES http_alert_sinks(id) ON DELETE CASCADE;
ALTER TABLE http_alert_sink_secrets ADD CONSTRAINT http_alert_sink_secrets_secret_id_fkey
    FOREIGN KEY (secret_id) REFERENCES secrets(id) ON DELETE RESTRICT;

ALTER TABLE sites ADD CONSTRAINT sites_source_id_fkey
    FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE RESTRICT;
ALTER TABLE sites ADD CONSTRAINT sites_http_alert_sink_id_fkey
    FOREIGN KEY (http_alert_sink_id) REFERENCES http_alert_sinks(id) ON DELETE RESTRICT;

ALTER TABLE jobs ADD CONSTRAINT jobs_site_id_fkey
    FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE SET NULL;
ALTER TABLE jobs ADD CONSTRAINT jobs_source_id_fkey
    FOREIGN KEY (source_id) REFERENCES sources(id) ON DELETE RESTRICT;

ALTER TABLE alerts ADD CONSTRAINT alerts_site_id_fkey
    FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE;

ALTER TABLE rules ADD CONSTRAINT rules_site_id_fkey
    FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE;

ALTER TABLE seen_domains ADD CONSTRAINT seen_domains_site_id_fkey
    FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE;

ALTER TABLE processed_files ADD CONSTRAINT processed_files_site_id_fkey
    FOREIGN KEY (site_id) REFERENCES sites(id) ON DELETE CASCADE;

-- ============================================================================
-- INDEXES
-- ============================================================================

-- Jobs indexes
CREATE INDEX IF NOT EXISTS idx_jobs_pending_browser ON jobs (priority DESC, scheduled_at, created_at)
    WHERE type = 'browser' AND status = 'pending';
CREATE INDEX IF NOT EXISTS idx_jobs_pending_rules ON jobs (priority DESC, scheduled_at, created_at)
    WHERE type = 'rules' AND status = 'pending';
CREATE INDEX IF NOT EXISTS idx_jobs_pending_alert ON jobs (priority DESC, scheduled_at, created_at)
    WHERE type = 'alert' AND status = 'pending';
CREATE INDEX IF NOT EXISTS idx_jobs_pending_secret_refresh ON jobs (priority DESC, scheduled_at, created_at)
    WHERE type = 'secret_refresh' AND status = 'pending';
CREATE INDEX IF NOT EXISTS idx_jobs_running ON jobs (lease_expires_at)
    WHERE status = 'running';
CREATE UNIQUE INDEX IF NOT EXISTS idx_jobs_scheduler_fire_key_unique ON jobs ((metadata->>'scheduler.fire_key'))
    WHERE metadata ? 'scheduler.fire_key';
CREATE INDEX IF NOT EXISTS idx_jobs_type_created_at_desc ON jobs (type, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS idx_jobs_site_created_at ON jobs (site_id, created_at DESC)
    WHERE site_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_jobs_source_created_at ON jobs (source_id, created_at DESC)
    WHERE source_id IS NOT NULL;

-- Events indexes
CREATE INDEX IF NOT EXISTS idx_events_processing_queue ON events (priority DESC, created_at)
    WHERE should_process = TRUE AND processed = FALSE;
CREATE INDEX IF NOT EXISTS idx_events_session ON events (session_id);

-- Sources indexes
CREATE UNIQUE INDEX IF NOT EXISTS sources_name_uq ON sources (name);

-- Source Secrets indexes
CREATE INDEX IF NOT EXISTS idx_source_secrets_source ON source_secrets (source_id);
CREATE INDEX IF NOT EXISTS idx_source_secrets_secret ON source_secrets (secret_id);

-- Secrets indexes
CREATE INDEX IF NOT EXISTS idx_secrets_refresh_due ON secrets (last_refreshed_at)
    WHERE refresh_enabled = TRUE;

-- HTTP Alert Sinks indexes
CREATE UNIQUE INDEX IF NOT EXISTS http_alert_sinks_name_uq ON http_alert_sinks (name);
CREATE INDEX IF NOT EXISTS idx_http_alert_sinks_created_at ON http_alert_sinks (created_at DESC, id DESC);

-- HTTP Alert Sink Secrets indexes
CREATE INDEX IF NOT EXISTS idx_http_alert_sink_secrets_sink ON http_alert_sink_secrets (http_alert_sink_id);
CREATE INDEX IF NOT EXISTS idx_http_alert_sink_secrets_secret ON http_alert_sink_secrets (secret_id);

-- Sites indexes
CREATE UNIQUE INDEX IF NOT EXISTS sites_name_uq ON sites (name);
CREATE INDEX IF NOT EXISTS idx_sites_source_id ON sites (source_id);
CREATE INDEX IF NOT EXISTS idx_sites_http_alert_sink_id ON sites (http_alert_sink_id);
CREATE INDEX IF NOT EXISTS idx_sites_created_at ON sites (created_at DESC, id DESC);

-- Alerts indexes
CREATE INDEX IF NOT EXISTS idx_alerts_site_fired_at ON alerts (site_id, fired_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_site_status_fired_at ON alerts (site_id, (resolved_at IS NULL), fired_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_unresolved ON alerts (site_id, fired_at DESC)
    WHERE resolved_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_alerts_severity_fired_at ON alerts (severity, fired_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_rule_type_fired_at ON alerts (rule_type, fired_at DESC);
CREATE INDEX IF NOT EXISTS idx_alerts_resolved_by ON alerts (resolved_by)
    WHERE resolved_by IS NOT NULL;

-- Rules indexes
CREATE INDEX IF NOT EXISTS idx_rules_site_enabled ON rules (site_id, enabled);
CREATE INDEX IF NOT EXISTS idx_rules_type_enabled ON rules (rule_type, enabled);
CREATE INDEX IF NOT EXISTS idx_rules_priority ON rules (priority)
    WHERE enabled = true;

-- Seen Domains indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_seen_domains_site_domain_scope ON seen_domains (site_id, domain, scope);
CREATE INDEX IF NOT EXISTS idx_seen_domains_domain ON seen_domains (domain);
CREATE INDEX IF NOT EXISTS idx_seen_domains_last_seen ON seen_domains (last_seen_at DESC);

-- Processed Files indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_processed_files_site_hash_scope ON processed_files (site_id, file_hash, scope);
CREATE INDEX IF NOT EXISTS idx_processed_files_hash ON processed_files (file_hash);
CREATE INDEX IF NOT EXISTS idx_processed_files_processed_at ON processed_files (processed_at DESC);

-- Domain Allowlists indexes
CREATE UNIQUE INDEX IF NOT EXISTS idx_domain_allowlists_unique_pattern ON domain_allowlists (scope, pattern, pattern_type);
CREATE INDEX IF NOT EXISTS idx_domain_allowlists_scope_enabled ON domain_allowlists (scope, enabled)
    WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_domain_allowlists_pattern_type ON domain_allowlists (pattern_type, enabled)
    WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_domain_allowlists_pattern ON domain_allowlists (pattern)
    WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_domain_allowlists_priority ON domain_allowlists (priority)
    WHERE enabled = true;
CREATE INDEX IF NOT EXISTS idx_domain_allowlists_scope_lookup ON domain_allowlists (scope, pattern_type, enabled, priority)
    WHERE enabled = true;

-- IOC Domains indexes
-- Note: UNIQUE (domain) constraint already creates a btree index, so no separate idx_ioc_domains_domain needed
CREATE INDEX IF NOT EXISTS idx_ioc_domains_source ON ioc_domains (source);
CREATE INDEX IF NOT EXISTS idx_ioc_domains_severity ON ioc_domains (severity);
CREATE INDEX IF NOT EXISTS idx_ioc_domains_expires ON ioc_domains (expires_at)
    WHERE expires_at IS NOT NULL;

-- IOCs indexes
CREATE INDEX IF NOT EXISTS idx_iocs_type ON iocs (type);
CREATE INDEX IF NOT EXISTS idx_iocs_enabled ON iocs (enabled);
CREATE UNIQUE INDEX IF NOT EXISTS ux_iocs_fqdn_lower_value ON iocs (lower(value))
    WHERE type = 'fqdn';
CREATE UNIQUE INDEX IF NOT EXISTS ux_iocs_ip_value ON iocs (value)
    WHERE type = 'ip';

-- IOCs trigram index for substring/ILIKE searches (optional; requires pg_trgm extension)
DO $$
DECLARE
  has_trgm boolean := EXISTS (SELECT 1 FROM pg_extension WHERE extname='pg_trgm');
BEGIN
  IF has_trgm THEN
    BEGIN
      EXECUTE 'CREATE INDEX IF NOT EXISTS idx_iocs_value_trgm ON iocs USING gin (lower(value) public.gin_trgm_ops)';
    EXCEPTION WHEN undefined_object OR invalid_schema_name THEN
      NULL;
    END;
  END IF;
END
$$;

-- ============================================================================
-- TRIGGERS
-- ============================================================================

-- Jobs updated_at trigger
DROP TRIGGER IF EXISTS trg_jobs_set_updated_at ON jobs;
CREATE TRIGGER trg_jobs_set_updated_at
    BEFORE UPDATE ON jobs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Secrets updated_at trigger
DROP TRIGGER IF EXISTS trg_secrets_set_updated_at ON secrets;
CREATE TRIGGER trg_secrets_set_updated_at
    BEFORE UPDATE ON secrets
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Sites updated_at trigger
DROP TRIGGER IF EXISTS trg_sites_set_updated_at ON sites;
CREATE TRIGGER trg_sites_set_updated_at
    BEFORE UPDATE ON sites
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Rules updated_at trigger
DROP TRIGGER IF EXISTS trg_rules_set_updated_at ON rules;
CREATE TRIGGER trg_rules_set_updated_at
    BEFORE UPDATE ON rules
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- Domain Allowlists updated_at trigger
DROP TRIGGER IF EXISTS trg_domain_allowlists_set_updated_at ON domain_allowlists;
CREATE TRIGGER trg_domain_allowlists_set_updated_at
    BEFORE UPDATE ON domain_allowlists
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- IOCs updated_at trigger
DROP TRIGGER IF EXISTS trg_iocs_set_updated_at ON iocs;
CREATE TRIGGER trg_iocs_set_updated_at
    BEFORE UPDATE ON iocs
    FOR EACH ROW
    EXECUTE FUNCTION set_updated_at();

-- ============================================================================
-- COMMENTS
-- ============================================================================

COMMENT ON COLUMN domain_allowlists.scope IS 'Scope context for allowlist entries. Multiple sites can share the same scope. Use ''global'' for entries that apply to all scopes.';
COMMENT ON COLUMN secrets.provider_script_path IS 'Path to executable script that fetches/refreshes the secret value';
COMMENT ON COLUMN secrets.env_config IS 'JSON object of environment variables passed to the provider script';
COMMENT ON COLUMN secrets.refresh_interval IS 'How often to refresh the secret (e.g., ''1 hour'', ''30 minutes'')';
COMMENT ON COLUMN secrets.last_refreshed_at IS 'Timestamp of the last successful or failed refresh attempt';
COMMENT ON COLUMN secrets.last_refresh_status IS 'Status of the last refresh attempt: success, failed, or pending';
COMMENT ON COLUMN secrets.last_refresh_error IS 'Error message from the last failed refresh attempt';
COMMENT ON COLUMN secrets.refresh_enabled IS 'Whether automatic refresh is enabled for this secret';
