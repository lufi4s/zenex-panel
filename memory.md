# Zenex Panel — Project Memory

Read this first. Update after every task (see CLAUDE.md).

## 1. Current Project State & Architecture Summary

Zenex: self-hosted, WordPress-only VPS management panel (Kinsta/EasyWP-style).
Customer brings the VPS; Zenex installs the stack and a Zenex Agent.

Monorepo (see `docs/architecture.md`, `docs/security.md`):

| Path | Lang | State |
|---|---|---|
| `apps/api` | Go | Skeleton: `/api/v1/health`, env config, HTTP middleware, RBAC matrix, job state machine. Own `go.mod`. |
| `apps/panel`, `apps/admin` | SvelteKit | Empty dirs. Not started. |
| `services/agent` | Go (Linux target) | `internal/executor` (argv-only allowlisted runner), `internal/nginx` (`nginx -t` gate before reload). Own `go.mod`. |
| `services/security-engine`, `migration-engine`, `provisioner` | Go | Empty dirs. Not started. |
| `packages/validation` | Go | Done: site slug (reserved names), DNS domain, compose slug+apex, Linux user, idempotency key. Used via `replace` by `services/agent`. Not yet imported by `apps/api`. |
| `packages/shared-types`, `security-rules`, `config-schema` | — | Empty. |
| `apps/api/migrations/0001_init.sql` | SQL | Full central schema (identity, RBAC, nodes, sites, domains, jobs, job_steps, job_logs, backups, security events, quarantine, migrations, SMTP, email outbox, alerts, settings, append-only audit_logs). |
| `infrastructure/{ansible,systemd,nginx,deployment}` | — | Empty. |
| `tests/*` | — | Empty. |

Key decisions:
- Go 1.24 minimum in `go.mod` files; toolchain installed locally is Go 1.27.0 (installed via winget this session, at `C:\Program Files\Go\bin`; PATH must be refreshed in new shells).
- No Docker for the WordPress runtime (per spec).
- Agent is Linux-only. Executor tests that need POSIX binaries skip on Windows.
- No git repo initialized yet. Do not commit unless the user asks.
- Node 24 and npm 11 are present (for SvelteKit later).

## 2. Completed Tasks & Recent Changes

Session 1 (foundation):
- Created monorepo directory layout per spec.
- `packages/validation/{go.mod,validation.go,validation_test.go}`
- `services/agent/go.mod`
- `services/agent/internal/executor/{executor.go,executor_test.go}`
- `services/agent/internal/nginx/{nginx.go,nginx_test.go}`
- `apps/api/go.mod`
- `apps/api/cmd/api/main.go`
- `apps/api/internal/config/{config.go,config_test.go}`
- `apps/api/internal/rbac/{rbac.go,rbac_test.go}`
- `apps/api/internal/jobs/{state.go,state_test.go}`
- `apps/api/internal/httpapi/{router.go,router_test.go,middleware.go,errors.go}`
- `apps/api/migrations/0001_init.sql`
- `docs/architecture.md`, `docs/security.md`
- `memory.md` (this file)

Verification run:
- `packages/validation`: `go test` passes (Windows).
- `apps/api`: `go vet` + `go test ./...` pass (Windows); `GOOS=linux go vet` passes.
- `services/agent`: `go vet` + `go test` pass (Windows, POSIX-only tests skipped); `GOOS=linux go vet` and `go test -c` for both packages compile.
- `0001_init.sql`: applied cleanly with `ON_ERROR_STOP=1` on PostgreSQL 16.x (Windows, scratch cluster on port 55432). Verified: `audit_logs` UPDATE and DELETE raise "append-only"; `jobs.status='dead'` accepted; FK on `sites.owner_user_id` enforced.

## 3. Pending / Next Tasks

Priority order:
1. (Done) Apply and verify `0001_init.sql` on PostgreSQL 16.
2. Wire `apps/api` to PostgreSQL (pgx), migrations runner, repositories for users/sessions/jobs/audit.
3. Auth: Argon2id hashing, sessions (token hash stored, not token), CSRF, login rate limit, TOTP, password reset, session revocation.
4. Audit log writer used by every mutating handler (append-only table already enforced).
5. Job worker: lease via `locked_by`/`locked_until`, retry via `jobs.NextAfterFailure`, step table for resumable provisioning.
6. Agent: mTLS client, node identity, command dispatch with allowlisted actions only, audit, `nginx.Reload` exposed through it.
7. Agent actions: useradd/groupadd for per-site UID/GID, PHP-FPM pool creation (`php-fpm -t` before reload), MariaDB DB+user creation, WP-CLI install, wp-config hardening, Redis config, Let's Encrypt via ACME, UFW/nftables rules, Fail2Ban.
8. Provisioner service implementing the 19-step sequence with rollback.
9. DNS: Zenex NS + wildcard `*.apex -> VPS IP`, optional Cloudflare API.
10. Security Engine: inotify queue, ClamAV, YARA, PHP rule packs (`packages/security-rules`), WP checksum verification, quarantine with evidence.
11. Backups: manual/scheduled, retention daily/weekly/monthly, object storage, verification, restore + post-restore scan.
12. SMTP: encrypted credentials, queued sending, templates, SPF/DKIM/DMARC guidance.
13. Migration engine: cPanel API / SSH, WordPress-only, resumable, DNS switch only with explicit approval.
14. Frontend `apps/panel` (SvelteKit + TS) and `apps/admin`, wired to real endpoints only.
15. Encryption key management for `*_enc` columns.
16. Infrastructure: systemd units, Ansible, Nginx templates, deployment scripts.
17. Test suites: `tests/security` (cross-site, traversal, injection, webshell), `tests/integration`, `tests/e2e`, failure tests (reboot, disk full, etc.).

## 4. Known Issues / Risks

- PostgreSQL local env: installed at `C:\Program Files\PostgreSQL\16\bin` (winget). The first install left `lib/` missing until the msiexec finished; if `initdb` says `$libdir/... No such file`, wait for installer to finish. The scratch cluster lives in the session scratchpad, not the project.
- SQL `sites.slug` CHECK does not enforce the reserved-name list; that lives in `packages/validation` only. Enforce in the API layer too, or add a reserved-name CHECK.
- `apps/api/internal/httpapi/errors.go` declares `ErrMethodNotAllowed` and `ErrBodyTooLarge` but no code path uses them yet (kept for upcoming handlers).
- `apps/api` does not import `packages/validation` yet; it will when site/domain handlers exist.
- `services/agent` Linux tests were compiled but not executed (no WSL distro installed on this machine).
- `services/agent` has no `main` yet; the control channel to the API does not exist.
- The `jobs` package defines `dead` as a state, and the SQL `jobs.status` CHECK includes `dead`. Keep them in sync.
- `apps/api` has no authentication yet. Every route is public except none exist beyond health. Do not deploy.
- Frontend not started; no UI exists yet.

## 5. Context for Resuming

- Working dir: `C:\Users\YoKina\Desktop\zenex_panel`. Not a git repo.
- Run Go tests: `export PATH="$PATH:/c/Program Files/Go/bin"` (bash) then `go test ./...` in each module dir (`packages/validation`, `services/agent`, `apps/api`).
- Each Go module uses a relative `replace` for `packages/validation` only in `services/agent`. Keep that path stable.
- CLAUDE.md requires: read this file before starting, update after every task, and final reply `Done ✨` only.

## 6. Session 2: GitHub + VPS bootstrap

- `infrastructure/deployment/install.sh`: Ubuntu 24.04 bootstrap (nginx, php-fpm, mariadb, redis, ufw default-deny with 22/80/443, fail2ban, certbot, Zenex dirs). Does NOT install the Agent (no signed release yet).
- `.gitignore`, `.gitattributes` (LF for *.sh/*.sql/*.go/*.md).
- Git repo initialized on `main`. Commits: `36f9db7` (foundation + install script), `bb9dd98` (gitattributes). Local git identity set to Saiful <saiful.ops@zenexcloud.com>.
- GitHub CLI installed (`C:\Program Files\GitHub CLI\gh.exe`). NOT logged in. Repo not yet created or pushed.
- Pending: `gh auth login` (user must run), create repo `zenex-panel`, push, share raw install URL: `https://raw.githubusercontent.com/<owner>/zenex-panel/main/infrastructure/deployment/install.sh`. Replace `<owner>` in install.sh header comment if it differs.

## 7. Session 3: one-command panel install

Built:
- `apps/api` now a real control plane: PostgreSQL (pgx v5.7.2), embedded migrations applied on `serve`, `create-admin` subcommand (reads email+password from stdin), Argon2id passwords (OWASP params), 12h sessions stored as SHA-256 hashes, HttpOnly/Secure/SameSite=Strict cookie, custom-header CSRF guard, account lock after 5 failures (15 min), per-IP login rate limit (10/min), audit rows for login/logout, `/api/v1/system/metrics` reading real /proc + statfs (Linux only; 503 elsewhere).
- Embedded UI: `apps/api/web/index.html`, `assets/app.js`, `assets/app.css`. Vanilla JS, no build step, CSP `script-src 'self'`. Temporary until SvelteKit `apps/panel` exists.
- TLS: API serves HTTPS itself (`ZENEX_TLS_CERT`/`ZENEX_TLS_KEY`). Production refuses to start without TLS and DB.
- `infrastructure/deployment/install.sh`: one command on Ubuntu 24.04. Installs PostgreSQL, golang-go (1.22), builds API from git, creates DB role + DB, self-signed cert for server IP, systemd unit `zenex-api` (hardened), UFW (22/80/443/8443), base web stack (nginx, php-fpm, mariadb, redis), admin account. Prints `https://<ip>:8443`. Credentials in `/root/zenex-admin-credentials.txt`. Re-run keeps secrets and admin.

Verified:
- `go test ./...` passes for apps/api (Windows). `GOOS=linux go build` and `go vet` pass.
- End-to-end on Windows against PostgreSQL 16 (scratch cluster port 55432, DB zenex_e2e): migrate, create-admin, login 200 with cookie, /me 200, metrics 503 (expected on Windows), logout 204, session revoked (401), audit rows written.
- `install.sh` passes `bash -n`. NOT run on a real Ubuntu server.

Not verified:
- Live /proc metrics output (needs Linux runtime; no WSL distro installed).
- Full installer on a fresh VPS.

Pending (in order):
1. Test install.sh on a throwaway Ubuntu 24.04 VPS; fix anything it hits.
2. Frontend: replace embedded UI with SvelteKit `apps/panel` once login flow is stable.
3. Agent: `main`, outbound mTLS registration, command dispatch via allowlist.
4. Site creation: DNS (wildcard), Linux user, PHP-FPM pool, MariaDB, WP-CLI install, Nginx vhost + `nginx -t` gate, SSL.
5. Password reset, TOTP 2FA, CSRF token (currently custom-header only), session list/revoke UI.
6. Let's Encrypt for panel domain (currently self-signed IP cert).

## 8. Session 4: WordPress website creation (single VPS)

Built:
- `services/agent/internal/helper` + `cmd/zenex-helper`: root daemon on `/run/zenex/helper.sock` (group zenex). Fixed operations only: user.create, fs.prepare, db.create (SQL on stdin), pool.write (php-fpm -t gate), vhost.write (nginx -t gate via nginx.Reload), wp.core-download / config-create / core-install / harden. All args validated; site account = `zx_<label>`.
- `apps/api/internal/provision`: 10 idempotent steps recorded in job_steps. A retry re-runs all steps (each checks state first). Passwords derived via HMAC(ZENEX_SECRET_KEY, site id), never stored. Error text scrubbed of passwords.
- `apps/api/internal/store/sites.go`, `internal/helperclient`, `internal/httpapi/sites.go`: domains, sites (idempotency key required), jobs (get/retry), site credentials (owner/admin only, audited).
- UI `apps/api/web`: Websites tab (connect domain, create site with live progress, retry, WordPress login).
- Installer: 15 steps; builds helper, installs WP-CLI after SHA-512 check, helper systemd unit, keeps ZENEX_SECRET_KEY across runs.

Verified on VPS 162.4.35.76 (end to end):
- Domain 162.4.35.76.nip.io connected; site shop1 created via API; job succeeded (10/10 steps).
- http://shop1.162.4.35.76.nip.io -> 200, wp-login 200, dotfiles 403.
- WordPress login with the panel-provided credentials -> 302 to wp-admin, wp-admin 200.

Bugs found and fixed during this session (keep in mind):
- site home dir must be traversable (root:root 0751), htdocs zx_<site>:www-data 0750.
- nginx snippets/fastcgi-php.conf already sets try_files -> do not repeat it.
- retry must not skip steps (earlier fixes were not applied).
- Installer: ZENEX_SECRET_KEY must survive reinstalls. If /etc/zenex/panel.env is lost, all site DB and WP admin passwords change and sites break. BACK THIS FILE UP.

Known limitations (not yet built):
- SSL / Let's Encrypt, real DNS automation, site delete, backups, malware scanning, PHP function restrictions, rollback (retry instead).
- wp-cli receives DB and WP admin passwords as command arguments (visible to root in process list for the duration of the command). Move to stdin/--prompt.
- Domain ownership is not verified (no DNS check) before a site is created.
- The API and helper share one socket group (zenex); any process running as zenex can request site operations.
- Panel must be served over HTTPS (self-signed IP cert); site HTTP only.

## 9. Session 5: DNS verification and site management

DNS verification (`apps/api/internal/dnscheck`):
- Connecting a domain probes a random name under it. Verified only when `*.domain` resolves to ZENEX_PUBLIC_IP (wildcard required). Apex-only is not enough. Messages tell the customer which A records to add; Cloudflare grey-cloud noted.
- Website creation requires the exact name (`label.domain`) to resolve to this server (400 `dns_not_pointing`).
- Endpoint `POST /api/v1/domains/{id}/verify` re-checks.

Site management (`apps/api/internal/manage`, helper ops in `services/agent/internal/helper/manage.go`):
- Suspend / resume (vhost disabled; data kept). Suspended addresses show 404.
- PHP-FPM restart for the site's version (affects other sites on that version).
- PHP version change (new pool validated before the old one is removed; only installed versions).
- Error log tail (`logs.tail`).
- Delete: background job `site.delete`, `site.purge` removes vhost, pool, database, DB user, home dir, logs and system account. Address/name become reusable (migration 0003 uses partial unique indexes on live rows).
- Every action is state-checked and audited.

Verified on VPS 162.4.35.76:
- example.com (behind Cloudflare, not this server) -> not verified, message with exact records; site creation refused.
- 162.4.35.76.nip.io -> verified.
- Suspend -> 404; resume -> 200. PHP restart 200; logs 200 (real nginx error lines); versions ["8.3"]; same-version switch 409; missing version 422.
- Delete shop1 -> job succeeded; files, account, DB, pool, vhost removed; address 404.
- Recreate shop1 with same name -> provisioned successfully.
- Unknown hosts -> 404 (installer replaces Ubuntu default site with zenex-default catch-all).

Not built (the "full manage" gap):
- Backups / restore, SSL, staging / clone, cache clear, WordPress updates, plugin/theme management, add domain aliases, resource limits, per-site traffic/health monitoring, malware scanning.

Operational notes:
- raw.githubusercontent.com caches briefly and different edges can serve different versions. If the VPS runs an old installer, run it from the git checkout: `git -C /opt/zenex/src fetch --depth 1 origin main && git -C /opt/zenex/src reset --hard FETCH_HEAD && bash /opt/zenex/src/infrastructure/deployment/install.sh`.

## 10. Session 6: Caddy replaces nginx (automatic HTTPS)

- Web server is now Caddy 2.6 (Ubuntu package). nginx is purged by the installer.
- Helper writes `/etc/caddy/zenex-available/zx-<user>.caddy`, enables it via symlink in `/etc/caddy/zenex`, and runs `caddy validate` before every reload (`services/agent/internal/caddy`). A failed validation removes the change.
- Site template: `root`, `php_fastcgi unix//run/php/zx-<user>.sock`, uploads PHP blocked (403), dotfiles blocked (403), per-site log `/var/log/caddy/zx-<user>.log`.
- Caddy obtains and renews Let's Encrypt certificates itself. Verified: `yokohama.ozima.cloud` and `caddytest.ozima.cloud` serve valid HTTPS (Let's Encrypt, ssl_verify=0). HTTP redirects to HTTPS (308).
- Unknown hosts: `:80 { respond 404 }` catch-all in `/etc/caddy/Caddyfile`.
- Bug fixed: PHP pool files must end in `.conf` (PHP-FPM ignores other names). Regression test added.
- Suspend 404 / resume restored verified on the server.
- Existing site `yokohama.ozima.cloud` was re-written to the Caddy config via helper `vhost.write`.
- Operations: the VPS has a real domain `ozima.cloud` with wildcard DNS; use it for SSL tests (nip.io shares a Let's Encrypt rate limit).

## 11. Multi-page panel (latest)

- Frontend is now a routed, multi-page panel: sidebar layout (AppLayout + app-sidebar), pages in apps/panel/src/pages: Overview, Websites, Website (SitePage with Overview / Files / Logs / Settings tabs), Domains, Server, Activity, Settings (admins only).
- Removed the old single-page files: Dashboard.tsx, WebsitesCard.tsx, AppShell.tsx.
- Checks passing locally: tsc -b, vp check (lint + format), vitest (33 tests), vite build into apps/api/web/dist, go vet and go test ./... in apps/api.
- Pushed to origin main as 707024e.
- NOT yet deployed to the VPS (162.4.35.76). Deploy needs ZENEX_VPS_PASS in the environment; run the install script from the git checkout on the VPS (`git -C /opt/zenex/src fetch -q --depth 1 origin main && git -C /opt/zenex/src reset -q --hard FETCH_HEAD && bash /opt/zenex/src/infrastructure/deployment/install.sh`).
- Not yet verified: visual/mobile check of the new pages in headless Chrome.

## 12. Create-website popup shows build status (latest)

- "New website" dialog (apps/panel/src/pages/WebsitesPage.tsx): the form is labelled "Subdomain name" (apps/panel/src/components/NewWebsiteCard.tsx). After "Create website" succeeds, the dialog stays open and shows JobProgress with the step list and the build log expanded, plus an "Open website" button. Closing the dialog does not stop the build.
- JobProgress (apps/panel/src/components/JobProgress.tsx) has a new optional prop defaultShowLog.
- Test added in apps/panel/src/app.test.tsx: "shows the build status and log in the popup after creating a website". 34 frontend tests pass; vp check, tsc and vite build pass.
- Not committed or deployed in this step.
