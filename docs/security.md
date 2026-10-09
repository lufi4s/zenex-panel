# Zenex Security Model

Zenex uses defense in depth. It does not promise that malware can never run on a website. It aims to limit what a
compromised website can reach, and to keep the panel itself small and hard to misuse.

## Implemented

**Accounts and sessions**
- Passwords hashed with Argon2id. An account locks for 15 minutes after 5 failed sign-ins. Sign-in is rate limited per IP.
- Session tokens are random; only their SHA-256 hash is stored. 12-hour sessions. The cookie is `HttpOnly`, `Secure`,
  `SameSite=Strict`. Logout revokes the session.
- Every state-changing request needs a custom header (CSRF guard). Roles are checked on every route (`internal/rbac`).
- Settings, panel update, restore from the backup server and cPanel migration are administrator only.

**Command execution**
- The API runs no system commands. A root helper on a Unix socket runs a fixed list of operations.
- Helper inputs are validated again (`packages/validation` and per-operation rules). Programs are allowlisted absolute
  paths, called with argument lists (no shell), a fixed environment, timeouts and output caps. Remote values used in a
  remote shell command (the cPanel migration) are single-quoted by a tested function.
- Archive restore checks every entry name (no absolute names, no `..`), refuses symbolic links, and unpacks in a root-only
  work folder. Uploaded files are staged in a folder only the panel account can write, and imported with an exclusive
  create (an existing file is never replaced).

**Website isolation**
- One Linux account, PHP-FPM pool, directory tree and database user per website. Files are 0750 / 0640.
- Uploaded PHP in `wp-content/uploads` is blocked; dotfiles are not served.
- The one-click WordPress sign-in uses a signed link (HMAC with a per-site key, valid 90 seconds, single use). The key is
  in a file readable only by that website's account.

**Secrets**
- Website database and admin passwords are derived from the master key and the site ID; they are not stored.
- Saved SMTP, Telegram and SFTP secrets are encrypted with AES-GCM, key derived from `ZENEX_SECRET_KEY`.
- The cPanel password is held in memory only, given to `ssh` through an environment variable, and removed from error text.
- Never logged: passwords, tokens, database passwords, private keys, session tokens, raw query strings.

**Network and browser**
- The panel serves HTTPS itself. Firewall: 22, 80, 443 and the panel port only. Fail2Ban is installed.
- Strict Content Security Policy, frame denial, `nosniff`, no-store caching, request body limit (1 MiB; 65 MiB for file
  uploads only), panic recovery.
- Migration and scan requests refuse loopback, link-local and unspecified hosts.

**Audit**
- `audit_logs` records sensitive actions and rejects UPDATE, DELETE and TRUNCATE at the database level.

## Not implemented yet

- Two-factor sign-in, password reset by email, a list of active sessions.
- Malware scanning (ClamAV, YARA), WordPress checksum verification, quarantine.
- PHP function restrictions per website; per-website resource limits.
- Per-user upload quota (a customer could fill the disk with uploads on their own website).
- Key rotation for the master secret.
- The `tests/security` adversarial suite (cross-site access, traversal, injection) is empty. The unit tests cover input
  validation and the helper's refusals, but there is no end-to-end attack test.
- The Server page and service status are visible to every signed-in user, including customers. Decide whether customers
  should see the server.
- The cPanel migration trusts the first SSH host key it sees (`accept-new`) and then pins it.

## Operating notes

- Back up `/etc/zenex/panel.env` (master key). Losing it changes every website's passwords.
- The panel's own certificate is self-signed until you put a domain and certificate on it.
