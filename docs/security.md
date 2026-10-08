# Zenex Security Model

Zenex uses defense in depth. It does not promise that malware is impossible to
run on a site; it aims to limit what a compromised site can reach and to detect
and contain compromise quickly.

## Implemented so far

- **No shell from untrusted input.** `services/agent/internal/executor` accepts only
  allowlisted absolute binaries, passes arguments as argv, rejects control
  characters, uses a fixed environment, and enforces timeouts and output caps.
- **Validated identifiers.** `packages/validation` checks site slugs (with reserved
  names), domains, Linux usernames, and idempotency keys before they reach a path,
  a user name, or a DNS record.
- **Config gate before reload.** `nginx -t` runs before every reload; a failure stops
  the reload.
- **RBAC matrix.** `apps/api/internal/rbac`. Support cannot hold node-root, agent,
  or secret permissions. Only Security Administrators release quarantine.
- **HTTP hardening.** Request IDs, no-store caching, CSP `default-src 'none'`,
  nosniff, frame denial, 1 MiB body limit, panic recovery, query strings omitted
  from access logs.
- **Append-only audit log.** `audit_logs` rejects UPDATE, DELETE, and TRUNCATE at the
  database level.
- **Encrypted-at-rest columns by name.** Secrets are stored as `*_enc` BYTEA columns.
  Encryption keys are not yet implemented (see pending work).

## Required before production

- Agent mTLS with per-node certificates and replay protection.
- Argon2id password hashing, TOTP, login rate limiting, session revocation.
- Key management for `*_enc` columns (KMS or sealed key on the control plane).
- Per-site UID/GID, PHP-FPM pools, and MariaDB users created by the Agent.
- Upload PHP execution blocking, PHP function restrictions after compatibility tests.
- ClamAV, YARA, WordPress checksum verification, and quarantine workflow.
- Adversarial test suite in `tests/security`.

## Never log

Passwords, API tokens, SMTP passwords, database passwords, Agent secrets, private keys,
session tokens, or raw query strings.
