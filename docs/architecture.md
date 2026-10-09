# Zenex Architecture

Zenex is a single-server control panel for WordPress. The panel and the websites it manages run on the same Ubuntu 24.04
machine. This document describes what is built today.

## Components

| Component | Language | Location | Role |
|---|---|---|---|
| Panel UI | TypeScript, SolidJS, Vite, Tailwind, Kobalte | `apps/panel` | Single-page app. Built into `apps/api/web/dist` and embedded in the API binary. |
| API | Go (standard library `net/http`, pgx) | `apps/api` | Sign-in, sessions, RBAC, websites, domains, backups, settings, jobs, monitoring, alerts, scheduler, panel update. Serves the UI. |
| Helper | Go, runs as root | `services/agent` (`cmd/zenex-helper`) | The only component that changes the system. Fixed operations on a Unix socket. |
| Validation | Go | `packages/validation` | Site names, domains, accounts, idempotency keys. Used by the API and the helper. |
| Database | PostgreSQL 16 | `apps/api/migrations` | Central data. Migrations are embedded and applied when the API starts. |
| Web server | Caddy 2 | configured by the helper | HTTPS, automatic certificates, one site file per website. |
| PHP | PHP-FPM, one pool per website | configured by the helper | Runs WordPress as the website's own Linux account. |
| Site databases | MariaDB | created by the helper | One database and user per website. |
| Installer | Bash | `infrastructure/deployment` | `install.sh` (one command, repeatable) and `update.sh` (panel update). |

Not built (early plans): a separate security engine (malware scanning), a separate migration engine, a multi-node agent
with mutual TLS. The `services/security-engine`, `migration-engine` and `provisioner` folders are empty.

## Process layout on the server

```
zenex-api.service     user "zenex"   HTTPS panel + API on the panel port (default 8443)
zenex-helper.service  root           Unix socket /run/zenex/helper.sock, group "zenex"
caddy.service                        ports 80 and 443 for the websites
postgresql, mariadb, php*-fpm, redis, fail2ban, ufw
```

## Trust boundaries

1. **Browser ↔ API.** HTTPS, session cookie (`HttpOnly`, `Secure`, `SameSite=Strict`), a CSRF header on every
   state-changing request, role checks on every route, request IDs, audit log.
2. **API ↔ Helper.** A Unix socket that only root and the `zenex` group can open. The API sends an operation name and
   string arguments. The helper validates every argument again. The API never runs system commands.
3. **Helper ↔ operating system.** Every external program is an allowlisted absolute path, started with an argument list
   (no shell), a fixed environment, a timeout and capped output. Secrets are passed through stdin or environment
   variables, not arguments, where the tool allows it.
4. **Site ↔ site.** Each website has its own Linux account, PHP-FPM pool and socket, directory tree
   (`/var/www/<account>/htdocs`, mode 0750), and MariaDB user. PHP in `wp-content/uploads` is blocked and dotfiles are not
   served.
5. **Panel ↔ remote servers.** SFTP backups and cPanel migration use `ssh`/`sftp` started by the helper, with
   `StrictHostKeyChecking=accept-new` and a known-hosts file owned by root.

## Jobs

Long work is a row in `jobs` with ordered `job_steps`, so the UI can show progress.

- `site.provision` (build a website, 10 steps) runs from the job queue and is **resumed** after a panel restart; each
  step checks its own state before acting, so a repeat is safe.
- `site.backup`, `site.restore`, `site.migrate`, `site.delete` run as goroutines inside the API process. If the panel
  restarts, they are marked **failed** at start-up (`Store.FailInterruptedJobs`) and can be started again.
- `GET /api/v1/sites/activity` reports, per website, the latest of these jobs while it runs and for 10 minutes after.

## Website build (provisioning)

create Linux account → prepare folders → create database and user → write PHP-FPM pool (`php-fpm -t` first) → write Caddy
site file (`caddy validate` first, then reload) → download WordPress → create `wp-config.php` → install WordPress →
harden → record the result. Passwords are derived from `ZENEX_SECRET_KEY` and the site ID (HMAC), so they are not stored.

## Backups, restore and migration

- **Archive format:** `tar.gz` with `htdocs/` and `database.sql`, plus a `<name>.json` manifest (domain, site ID, account,
  database, time) next to it. Backups on an SFTP server can therefore be found by another panel.
- **Restore:** check the archive entries, unpack into a root-only work folder, move the old `htdocs` aside, put the new one
  in place, give all files to the site account, drop and reload the database, write `wp-config.php` again with this site's
  database account, change stored addresses if the domain differs. If the database step fails the files are put back.
- **cPanel migration:** the helper reads the account over SSH only (find WordPress, stream `tar` and `mysqldump` into the
  same archive format), then the normal restore runs. Nothing is written on the cPanel side and DNS is never changed.

## Ordering rules for web-server changes

PHP-FPM pool files are tested with `php-fpm -t` and Caddy files with `caddy validate` before any reload. A failed check
removes the change. This is in the helper and cannot be bypassed through the API. All other helper operations run one at
a time under a lock; long copies from another server (`cpanel.scan`, `cpanel.pull`) do not take the lock.

## Data

Main tables: users, roles and permissions, sessions, login attempts, vps_nodes, sites, domains and DNS zones, jobs,
job_steps, job_logs, backups, notifications, site health and host metrics, settings, audit_logs (append-only).
