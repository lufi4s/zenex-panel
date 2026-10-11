-- Migration 0010: per-website page cache (saved pages served without PHP).

ALTER TABLE sites ADD COLUMN IF NOT EXISTS page_cache BOOLEAN NOT NULL DEFAULT false;
