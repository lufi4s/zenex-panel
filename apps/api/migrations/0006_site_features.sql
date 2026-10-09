-- Migration 0006: WordPress auto-updates, maintenance mode and the backup catalogue.

ALTER TABLE sites ADD COLUMN IF NOT EXISTS auto_update BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE sites ADD COLUMN IF NOT EXISTS maintenance BOOLEAN NOT NULL DEFAULT false;

-- Migration 0001 created a placeholder "backups" table that no code ever wrote to.
-- It is kept under another name so nothing is dropped, and the new catalogue
-- takes the name "backups".
ALTER TABLE IF EXISTS backups RENAME TO backups_legacy_0001;

CREATE TABLE IF NOT EXISTS backups (
    id          BIGSERIAL PRIMARY KEY,
    site_id     UUID NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    size_bytes  BIGINT NOT NULL CHECK (size_bytes >= 0),
    path        TEXT NOT NULL CHECK (length(path) BETWEEN 1 AND 1024)
);
CREATE INDEX IF NOT EXISTS backups_site_created_idx ON backups (site_id, created_at DESC);
