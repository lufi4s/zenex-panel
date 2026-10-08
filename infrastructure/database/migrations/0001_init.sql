-- Zenex central database, migration 0001.
-- Target: PostgreSQL 16+. Site databases (MariaDB) live on the VPS, not here.
-- Secrets (SMTP passwords, TOTP seeds, agent keys) are stored encrypted by the
-- application; this schema only holds ciphertext columns named *_enc.

CREATE EXTENSION IF NOT EXISTS citext;

-- ---------------------------------------------------------------------------
-- Identity & RBAC
-- ---------------------------------------------------------------------------

CREATE TABLE roles (
    id          SMALLSERIAL PRIMARY KEY,
    name        TEXT NOT NULL UNIQUE CHECK (name IN ('customer', 'support', 'administrator', 'security_administrator'))
);

CREATE TABLE permissions (
    id          SMALLSERIAL PRIMARY KEY,
    code        TEXT NOT NULL UNIQUE CHECK (code ~ '^[a-z_]+\.[a-z_]+$')
);

CREATE TABLE role_permissions (
    role_id       SMALLINT NOT NULL REFERENCES roles (id) ON DELETE CASCADE,
    permission_id SMALLINT NOT NULL REFERENCES permissions (id) ON DELETE CASCADE,
    PRIMARY KEY (role_id, permission_id)
);

CREATE TABLE users (
    id                 UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    email              CITEXT NOT NULL UNIQUE,
    password_hash      TEXT NOT NULL,              -- Argon2id PHC string
    totp_secret_enc    BYTEA,                      -- encrypted at rest; NULL until 2FA enabled
    totp_enabled       BOOLEAN NOT NULL DEFAULT FALSE,
    status             TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'locked', 'disabled')),
    failed_logins      INTEGER NOT NULL DEFAULT 0 CHECK (failed_logins >= 0),
    locked_until       TIMESTAMPTZ,
    last_login_at      TIMESTAMPTZ,
    created_at         TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_roles (
    user_id  UUID     NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    role_id  SMALLINT NOT NULL REFERENCES roles (id) ON DELETE RESTRICT,
    PRIMARY KEY (user_id, role_id)
);

CREATE TABLE sessions (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id       UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash    BYTEA NOT NULL UNIQUE,           -- SHA-256 of the session token; token itself never stored
    ip_address    INET,
    user_agent    TEXT,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at    TIMESTAMPTZ NOT NULL,
    revoked_at    TIMESTAMPTZ
);
CREATE INDEX sessions_user_idx ON sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE password_resets (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    token_hash  BYTEA NOT NULL UNIQUE,
    expires_at  TIMESTAMPTZ NOT NULL,
    used_at     TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Infrastructure
-- ---------------------------------------------------------------------------

CREATE TABLE vps_nodes (
    id                   UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id        UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    name                 TEXT NOT NULL CHECK (length(name) BETWEEN 1 AND 100),
    public_ip            INET NOT NULL,
    os_release           TEXT,                     -- e.g. "ubuntu-24.04"
    agent_version        TEXT,
    agent_cert_fingerprint BYTEA UNIQUE,           -- SHA-256 of the agent client certificate public key
    status               TEXT NOT NULL DEFAULT 'pending'
                         CHECK (status IN ('pending', 'online', 'degraded', 'offline', 'decommissioned')),
    last_seen_at         TIMESTAMPTZ,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE dns_zones (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id   UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    apex_domain     CITEXT NOT NULL UNIQUE,         -- customer's primary domain, e.g. example.com
    mode            TEXT NOT NULL CHECK (mode IN ('zenex_ns', 'cloudflare_api', 'external')),
    verified_at     TIMESTAMPTZ,
    cf_zone_id      TEXT,
    cf_token_enc    BYTEA,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Sites
-- ---------------------------------------------------------------------------

CREATE TABLE sites (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id   UUID NOT NULL REFERENCES users (id) ON DELETE RESTRICT,
    node_id         UUID NOT NULL REFERENCES vps_nodes (id) ON DELETE RESTRICT,
    zone_id         UUID REFERENCES dns_zones (id) ON DELETE SET NULL,
    slug            TEXT NOT NULL CHECK (slug ~ '^[a-z][a-z0-9-]{1,30}[a-z0-9]$'),
    primary_domain  CITEXT NOT NULL UNIQUE,         -- e.g. shop.example.com
    linux_user      TEXT NOT NULL UNIQUE CHECK (linux_user ~ '^[a-z_][a-z0-9_-]{0,31}$'),
    linux_uid       INTEGER UNIQUE,                 -- allocated by the agent, not the panel
    php_version     TEXT NOT NULL,
    db_name         TEXT NOT NULL UNIQUE,
    db_user         TEXT NOT NULL UNIQUE,
    -- Lifecycle state. Only the provisioner moves a site out of 'provisioning'.
    state           TEXT NOT NULL DEFAULT 'provisioning'
                    CHECK (state IN ('provisioning', 'ready', 'degraded', 'suspended',
                                     'quarantined', 'migrating', 'failed', 'deleting', 'deleted')),
    health          TEXT NOT NULL DEFAULT 'unknown' CHECK (health IN ('unknown', 'healthy', 'warning', 'critical')),
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ,
    UNIQUE (node_id, slug)
);
CREATE INDEX sites_owner_idx ON sites (owner_user_id) WHERE deleted_at IS NULL;

CREATE TABLE domains (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id     UUID NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    fqdn        CITEXT NOT NULL UNIQUE,
    kind        TEXT NOT NULL CHECK (kind IN ('primary', 'alias', 'redirect')),
    ssl_state   TEXT NOT NULL DEFAULT 'none' CHECK (ssl_state IN ('none', 'pending', 'active', 'failed', 'expiring')),
    ssl_expires_at TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Jobs (durable, resumable, idempotent)
-- ---------------------------------------------------------------------------

CREATE TABLE jobs (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    type             TEXT NOT NULL,                  -- e.g. site.provision, backup.run
    status           TEXT NOT NULL DEFAULT 'queued'
                     CHECK (status IN ('queued', 'running', 'succeeded', 'failed', 'cancelled', 'dead')),
    actor_user_id    UUID REFERENCES users (id) ON DELETE SET NULL,
    node_id          UUID REFERENCES vps_nodes (id) ON DELETE SET NULL,
    site_id          UUID REFERENCES sites (id) ON DELETE SET NULL,
    idempotency_key  TEXT,
    payload          JSONB NOT NULL DEFAULT '{}'::jsonb,   -- must never contain secrets
    result           JSONB,
    error_code       TEXT,
    error_message    TEXT,                           -- sanitized; no secrets or raw stderr
    attempts         INTEGER NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    max_attempts     INTEGER NOT NULL DEFAULT 3 CHECK (max_attempts BETWEEN 1 AND 10),
    locked_by        TEXT,
    locked_until     TIMESTAMPTZ,
    run_after        TIMESTAMPTZ NOT NULL DEFAULT now(),
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    started_at       TIMESTAMPTZ,
    finished_at      TIMESTAMPTZ,
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (actor_user_id, idempotency_key)
);
CREATE INDEX jobs_ready_idx ON jobs (run_after) WHERE status = 'queued';
CREATE INDEX jobs_site_idx ON jobs (site_id, created_at DESC);

CREATE TABLE job_steps (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    job_id       UUID NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    seq          INTEGER NOT NULL CHECK (seq >= 0),
    name         TEXT NOT NULL,                       -- e.g. create_linux_user
    status       TEXT NOT NULL DEFAULT 'pending'
                 CHECK (status IN ('pending', 'running', 'succeeded', 'failed', 'skipped', 'rolled_back')),
    attempts     INTEGER NOT NULL DEFAULT 0,
    error_code   TEXT,
    error_message TEXT,
    started_at   TIMESTAMPTZ,
    finished_at  TIMESTAMPTZ,
    UNIQUE (job_id, seq),
    UNIQUE (job_id, name)
);

CREATE TABLE job_logs (
    id          BIGSERIAL PRIMARY KEY,
    job_id      UUID NOT NULL REFERENCES jobs (id) ON DELETE CASCADE,
    level       TEXT NOT NULL CHECK (level IN ('info', 'warn', 'error')),
    message     TEXT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX job_logs_job_idx ON job_logs (job_id, id);

-- ---------------------------------------------------------------------------
-- Backups
-- ---------------------------------------------------------------------------

CREATE TABLE backups (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id         UUID NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    job_id          UUID REFERENCES jobs (id) ON DELETE SET NULL,
    kind            TEXT NOT NULL CHECK (kind IN ('manual', 'daily', 'weekly', 'monthly', 'pre_restore', 'pre_migration')),
    status          TEXT NOT NULL CHECK (status IN ('running', 'succeeded', 'failed', 'verified', 'expired')),
    storage_key     TEXT,                           -- object-storage key; path is never user-supplied
    sha256          BYTEA,
    size_bytes      BIGINT CHECK (size_bytes IS NULL OR size_bytes >= 0),
    immutable_until TIMESTAMPTZ,                    -- object-lock retention
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    verified_at     TIMESTAMPTZ
);
CREATE INDEX backups_site_idx ON backups (site_id, created_at DESC);

-- ---------------------------------------------------------------------------
-- Security
-- ---------------------------------------------------------------------------

CREATE TABLE security_events (
    id           BIGSERIAL PRIMARY KEY,
    site_id      UUID REFERENCES sites (id) ON DELETE SET NULL,
    node_id      UUID REFERENCES vps_nodes (id) ON DELETE SET NULL,
    severity     TEXT NOT NULL CHECK (severity IN ('INFO', 'LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    confidence   SMALLINT NOT NULL CHECK (confidence BETWEEN 0 AND 100),
    rule_id      TEXT,
    rule_version TEXT,
    category     TEXT NOT NULL,                    -- webshell, integrity, outbound, process, ...
    file_path    TEXT,
    description  TEXT NOT NULL,
    evidence     JSONB NOT NULL DEFAULT '{}'::jsonb,
    action_taken TEXT NOT NULL DEFAULT 'none' CHECK (action_taken IN ('none', 'alert', 'quarantine', 'pool_stopped', 'network_restricted')),
    status       TEXT NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'acknowledged', 'false_positive', 'resolved')),
    detected_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX security_events_site_idx ON security_events (site_id, detected_at DESC);

CREATE TABLE quarantine_items (
    id               UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    site_id          UUID NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    security_event_id BIGINT REFERENCES security_events (id) ON DELETE SET NULL,
    original_path    TEXT NOT NULL,
    sha256           BYTEA NOT NULL,
    rule_id          TEXT NOT NULL,
    severity         TEXT NOT NULL CHECK (severity IN ('INFO', 'LOW', 'MEDIUM', 'HIGH', 'CRITICAL')),
    evidence         JSONB NOT NULL DEFAULT '{}'::jsonb,
    storage_key      TEXT NOT NULL,                 -- where the quarantined bytes live
    status           TEXT NOT NULL DEFAULT 'held' CHECK (status IN ('held', 'released', 'purged')),
    quarantined_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    released_by      UUID REFERENCES users (id) ON DELETE SET NULL,
    released_at      TIMESTAMPTZ
);

-- ---------------------------------------------------------------------------
-- Migration
-- ---------------------------------------------------------------------------

CREATE TABLE migrations (
    id                UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id     UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    source_type       TEXT NOT NULL CHECK (source_type IN ('cpanel_api', 'ssh')),
    source_host      TEXT NOT NULL,
    source_credentials_enc BYTEA,                   -- encrypted; never returned to clients
    target_site_id    UUID REFERENCES sites (id) ON DELETE SET NULL,
    stage             TEXT NOT NULL DEFAULT 'connect',
    status            TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'running', 'paused', 'awaiting_approval', 'succeeded', 'failed', 'rolled_back')),
    dns_switch_approved_at TIMESTAMPTZ,             -- set only by explicit customer action
    rollback_info     JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at        TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Notifications & SMTP
-- ---------------------------------------------------------------------------

CREATE TABLE smtp_settings (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    owner_user_id   UUID NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    provider        TEXT NOT NULL CHECK (provider IN ('gmail', 'custom')),
    host            TEXT NOT NULL,
    port            INTEGER NOT NULL CHECK (port BETWEEN 1 AND 65535),
    encryption      TEXT NOT NULL CHECK (encryption IN ('starttls', 'tls', 'none')),
    username        TEXT,
    password_enc    BYTEA,                          -- encrypted; masked in UI, never logged
    from_name       TEXT NOT NULL,
    from_address    CITEXT NOT NULL,
    reply_to        CITEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (owner_user_id)
);

CREATE TABLE email_outbox (
    id              BIGSERIAL PRIMARY KEY,
    recipient       CITEXT NOT NULL,
    template        TEXT NOT NULL,
    template_vars   JSONB NOT NULL DEFAULT '{}'::jsonb,
    message_id      TEXT NOT NULL UNIQUE,
    status          TEXT NOT NULL DEFAULT 'queued' CHECK (status IN ('queued', 'sending', 'sent', 'failed')),
    attempts        INTEGER NOT NULL DEFAULT 0,
    last_error      TEXT,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    sent_at         TIMESTAMPTZ
);

-- ---------------------------------------------------------------------------
-- Alerts & system settings
-- ---------------------------------------------------------------------------

CREATE TABLE alerts (
    id           BIGSERIAL PRIMARY KEY,
    node_id      UUID REFERENCES vps_nodes (id) ON DELETE CASCADE,
    site_id      UUID REFERENCES sites (id) ON DELETE CASCADE,
    severity     TEXT NOT NULL CHECK (severity IN ('info', 'warning', 'critical')),
    kind         TEXT NOT NULL,                     -- disk_full, service_down, backup_failed, ...
    message      TEXT NOT NULL,
    raised_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    resolved_at  TIMESTAMPTZ
);
CREATE INDEX alerts_open_idx ON alerts (raised_at DESC) WHERE resolved_at IS NULL;

CREATE TABLE system_settings (
    key         TEXT PRIMARY KEY CHECK (key ~ '^[a-z_.]+$'),
    value       JSONB NOT NULL,
    updated_by  UUID REFERENCES users (id) ON DELETE SET NULL,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- ---------------------------------------------------------------------------
-- Audit log: append-only from the application's perspective.
-- Enforced in the database: UPDATE and DELETE are rejected by trigger.
-- ---------------------------------------------------------------------------

CREATE TABLE audit_logs (
    id              BIGSERIAL PRIMARY KEY,
    occurred_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    actor_user_id   UUID REFERENCES users (id) ON DELETE SET NULL,
    actor_role      TEXT,
    action          TEXT NOT NULL,                  -- e.g. site.create, dns.change, login.success
    target_type     TEXT,
    target_id       TEXT,
    node_id         UUID,
    site_id         UUID,
    job_id          UUID,
    ip_address      INET,
    result          TEXT NOT NULL CHECK (result IN ('success', 'failure', 'denied')),
    error_code      TEXT,
    metadata        JSONB NOT NULL DEFAULT '{}'::jsonb   -- must never contain secrets
);
CREATE INDEX audit_logs_actor_idx ON audit_logs (actor_user_id, occurred_at DESC);
CREATE INDEX audit_logs_target_idx ON audit_logs (target_type, target_id, occurred_at DESC);

CREATE OR REPLACE FUNCTION audit_logs_immutable() RETURNS trigger AS $$
BEGIN
    RAISE EXCEPTION 'audit_logs is append-only' USING ERRCODE = 'insufficient_privilege';
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER audit_logs_no_update
    BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW EXECUTE FUNCTION audit_logs_immutable();

CREATE TRIGGER audit_logs_no_truncate
    BEFORE TRUNCATE ON audit_logs
    FOR EACH STATEMENT EXECUTE FUNCTION audit_logs_immutable();

-- ---------------------------------------------------------------------------
-- Seed reference data
-- ---------------------------------------------------------------------------

INSERT INTO roles (name) VALUES
    ('customer'), ('support'), ('administrator'), ('security_administrator');
