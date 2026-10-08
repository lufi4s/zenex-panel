-- Migration 0002: record the result of DNS checks for connected domains.
-- verified_at (already in 0001) is set only when the wildcard record points here.

ALTER TABLE dns_zones
    ADD COLUMN IF NOT EXISTS last_checked_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS check_message   TEXT;
