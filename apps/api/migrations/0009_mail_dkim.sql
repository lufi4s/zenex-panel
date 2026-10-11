-- Migration 0009: the public DKIM value of a hosted mail domain. The private key stays on the
-- server (root only); only the public part is stored, to show it in the DNS records.

ALTER TABLE mail_domains ADD COLUMN IF NOT EXISTS dkim_public TEXT NOT NULL DEFAULT ''
    CHECK (length(dkim_public) <= 1024);
