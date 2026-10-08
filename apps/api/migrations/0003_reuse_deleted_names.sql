-- Migration 0003: a deleted website must free its name, address, account and
-- database for reuse. Uniqueness now applies only to live (non-deleted) rows.

ALTER TABLE sites DROP CONSTRAINT IF EXISTS sites_primary_domain_key;
ALTER TABLE sites DROP CONSTRAINT IF EXISTS sites_linux_user_key;
ALTER TABLE sites DROP CONSTRAINT IF EXISTS sites_linux_uid_key;
ALTER TABLE sites DROP CONSTRAINT IF EXISTS sites_db_name_key;
ALTER TABLE sites DROP CONSTRAINT IF EXISTS sites_db_user_key;
ALTER TABLE sites DROP CONSTRAINT IF EXISTS sites_node_id_slug_key;

CREATE UNIQUE INDEX IF NOT EXISTS sites_primary_domain_live ON sites (primary_domain) WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS sites_linux_user_live     ON sites (linux_user)     WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS sites_linux_uid_live      ON sites (linux_uid)      WHERE deleted_at IS NULL AND linux_uid IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS sites_db_name_live        ON sites (db_name)        WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS sites_db_user_live        ON sites (db_user)        WHERE deleted_at IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS sites_node_slug_live      ON sites (node_id, slug)  WHERE deleted_at IS NULL;

ALTER TABLE domains DROP CONSTRAINT IF EXISTS domains_fqdn_key;
CREATE UNIQUE INDEX IF NOT EXISTS domains_fqdn_live ON domains (fqdn);
