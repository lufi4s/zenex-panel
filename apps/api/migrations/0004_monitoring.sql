-- Migration 0004: history for host metrics and website uptime checks.
-- Rows older than the retention window are removed by the monitor.

CREATE TABLE IF NOT EXISTS metric_samples (
    ts               TIMESTAMPTZ NOT NULL,
    cpu_count        INTEGER NOT NULL CHECK (cpu_count > 0),
    load_1m          DOUBLE PRECISION NOT NULL CHECK (load_1m >= 0),
    mem_used_bytes   BIGINT NOT NULL CHECK (mem_used_bytes >= 0),
    mem_total_bytes  BIGINT NOT NULL CHECK (mem_total_bytes >= 0),
    disk_used_bytes  BIGINT NOT NULL CHECK (disk_used_bytes >= 0),
    disk_total_bytes BIGINT NOT NULL CHECK (disk_total_bytes >= 0)
);
CREATE INDEX IF NOT EXISTS metric_samples_ts_idx ON metric_samples (ts DESC);

CREATE TABLE IF NOT EXISTS site_checks (
    id          BIGSERIAL PRIMARY KEY,
    ts          TIMESTAMPTZ NOT NULL DEFAULT now(),
    site_id     UUID NOT NULL REFERENCES sites (id) ON DELETE CASCADE,
    ok          BOOLEAN NOT NULL,
    status_code INTEGER,
    latency_ms  INTEGER CHECK (latency_ms IS NULL OR latency_ms >= 0),
    error       TEXT
);
CREATE INDEX IF NOT EXISTS site_checks_site_ts_idx ON site_checks (site_id, ts DESC);
