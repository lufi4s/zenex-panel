-- Migration 0007: per-website SFTP access. The SFTP password is derived from the
-- site ID and sftp_generation (never stored); bumping the generation rotates it.

ALTER TABLE sites ADD COLUMN IF NOT EXISTS sftp_enabled BOOLEAN NOT NULL DEFAULT false;
ALTER TABLE sites ADD COLUMN IF NOT EXISTS sftp_generation INTEGER NOT NULL DEFAULT 0 CHECK (sftp_generation >= 0);
