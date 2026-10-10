-- Migration 0008: mailboxes on connected domains (phase 1: stored only; the mail server reads
-- them in phase 2). Passwords are stored as Argon2id hashes, never in plain text.

CREATE TABLE IF NOT EXISTS mail_domains (
    zone_id     UUID PRIMARY KEY REFERENCES dns_zones (id) ON DELETE CASCADE,
    enabled     BOOLEAN NOT NULL DEFAULT false,
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS mailboxes (
    id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    zone_id       UUID NOT NULL REFERENCES dns_zones (id) ON DELETE CASCADE,
    local_part    TEXT NOT NULL CHECK (local_part ~ '^[a-z0-9][a-z0-9._-]{0,63}$'),
    password_hash TEXT NOT NULL CHECK (length(password_hash) BETWEEN 20 AND 512),
    quota_mb      INTEGER NOT NULL DEFAULT 1024 CHECK (quota_mb BETWEEN 10 AND 102400),
    active        BOOLEAN NOT NULL DEFAULT true,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (zone_id, local_part)
);
CREATE INDEX IF NOT EXISTS mailboxes_zone_idx ON mailboxes (zone_id);
