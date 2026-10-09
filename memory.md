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

## 13. Deploy status

- GitHub main is at 532a16c (multi-page panel + create-website popup status). Pushed.
- VPS (162.4.35.76) is still on the old single-page build. Not deployed; user will run the update themselves later.
- Deploy command: `git -C /opt/zenex/src fetch -q --depth 1 origin main && git -C /opt/zenex/src reset -q --hard FETCH_HEAD && bash /opt/zenex/src/infrastructure/deployment/install.sh`
- Root VPS password was pasted in chat earlier; rotation recommended, not yet confirmed.

## 14. React Bits components (latest)

Installed via shadcn from reactbits.dev (`npx shadcn add https://reactbits.dev/r/<Name>-TS-TW`). Used where they fit:
- **Silk** (backgrounds) — `src/components/Silk.tsx`, lazy-loaded in `src/pages/LoginPage.tsx` as the sign-in aside background, tinted with the branding primary colour. Needs WebGL; mocked in tests.
- **SplitText** (text animations) — `src/components/SplitText.tsx`, used for the sign-in aside tagline (`LoginPage.tsx`). Not used for "Welcome back" so the text stays searchable in tests.
- **CountUp** (text animations) — `src/components/CountUp.tsx`, memory and disk percentages in `src/components/ServerStrip.tsx`. Uses IntersectionObserver; stubbed in tests.
- **SpotlightCard** (components) — `src/components/SpotlightCard.tsx`, wraps each stat tile in `ServerStrip.tsx` (light theme).

Removed (not a fit for any screen): `src/components/Stepper.tsx`, `src/components/AnimatedList.tsx`.

Dependencies added by the registry: gsap, @gsap/react, three, @react-three/fiber, motion, @types/three (dev), plus shadcn extras (@base-ui/react, cn, @fontsource-variable/geist, tw-animate-css).

Not replaced (no React Bits equivalent for the job): LineChart, Terminal, FileManager, DomainsCard, ActivityCard, NotificationBell, app-sidebar, shadcn ui primitives.

Checks: vp check (0 warnings), tsc -b clean, vitest 34/34, vite build OK. Visual check in browser not done yet.
Not committed/deployed in the first pass; see the commit that follows this section.

## 15. VPS deployed

- VPS (162.4.35.76) updated to commit 532a16c/acaf239 via install.sh; log at /root/zenex-run01.log on the VPS.
- Verified: zenex-api, zenex-helper and caddy active; https://127.0.0.1:8443 /, /websites, /login return 200; served bundle index-9F6vqxRE.js includes the Silk chunk.
- Root password was shared in chat again on this date; rotation still recommended.

## 16. StatusMark in the build progress

- Installed React Bits StatusMark via `npx shadcn@latest add @react-bits/StatusMark-JS-CSS` → `apps/panel/src/components/StatusMark.jsx` and `StatusMark.css`.
- Added `apps/panel/src/components/StatusMark.d.ts` so the JS component types in the TypeScript project.
- `apps/panel/src/components/JobProgress.tsx`: each build step's icon is now a StatusMark (succeeded→done, running→running, failed→failed, pending/skipped→pending). The lucide icon map was removed from this file.
- Checks: vp check clean, tsc clean, vitest 34/34, vite build OK.
- Not deployed to the VPS in this step.

## 17. In-panel update (in progress, NOT yet verified)

Goal: admins see "new version" and click "Update now" in the panel; the panel updates itself.

Written, not compiled or tested (Bash was denied by the auto-mode classifier in this session):
- `infrastructure/deployment/update.sh`: fetch+reset origin/main, then runs install.sh. Writes `/var/log/zenex/update.log`; last line `== update finished OK` or `== update FAILED`.
- `services/agent/internal/helper/update.go`: ops `panel.version`, `panel.latest` (git ls-remote), `panel.update-start` (systemd-run --unit=zenex-update --collect), `panel.update-status` (JSON state+log). Wired in `ops.go` dispatcher; binaries git, systemd-run, bash added to `AllowedBinaries`.
- `apps/api/internal/update/update.go`: Service.Status / Start (ErrRunning).
- `apps/api/internal/httpapi/updates.go`: GET and POST `/api/v1/system/update` (admin only; POST is CSRF-protected and audited as `panel.update`). Routes in `router.go`; Deps.Updates wired in `cmd/api/main.go`.
- `apps/panel`: `useSystemUpdate` / `useStartUpdate` in `api/queries.ts`; `components/UpdateCard.tsx` on Settings; admin banner on Overview; test "offers the panel update on the settings page" in `app.test.tsx`.

To verify before commit: `go vet ./... && go test ./...` in services/agent and apps/api; `vp check --fix`; `npx tsc -b`; `npx vitest run`; `npx vite build`.
Note: the first run of this feature must be deployed manually (the in-panel button does not exist on the VPS yet). The update runs `git reset --hard` on /opt/zenex/src and runs install.sh as root; admin-only and audited.

## 18. Plain React + Tailwind frontend (latest)

Removed every UI/animation/data/routing library from apps/panel. Only React, ReactDOM and Tailwind (plus Vite, Vitest and Testing Library for dev) remain.

- Data: `src/api/query.ts` is an in-house cache with the same hook names (`useQuery`, `useMutation`, `useInfiniteQuery`, `queryClient.invalidateQueries/setQueryData/clear`). `src/api/queries.ts` keeps its hook API; only its imports changed.
- Routing: `src/lib/router.tsx` (history API, `Link`, `NavLink`, `Navigate`, `Routes`, `useParams`, `useLocation`, `useNavigate`). `src/App.tsx` has a flat route table.
- UI primitives: `src/components/ui/*` rewritten in plain Tailwind (button, card, badge, alert, input, label, separator, skeleton, dialog, tabs). Radix/shadcn/sidebar/sheet/tooltip/avatar/dropdown removed.
- Icons: `src/components/icons.tsx` (inline SVG). lucide removed.
- Layout: `src/components/app-sidebar.tsx` and `src/layouts/AppLayout.tsx`, a fixed sidebar on large screens and a drawer on small screens.
- Typography: system sans for text, Georgia-style serif for headings, system monospace for code (`src/index.css`). No font packages, so the CSP stays strict.
- React Bits components removed (Silk, SplitText, CountUp, SpotlightCard, StatusMark). JobProgress uses a small inline `StepIcon`.
- Packages removed: @tanstack/react-query, react-router, lucide-react, radix-ui, @base-ui/react, class-variance-authority, clsx, cn, tailwind-merge, gsap, @gsap/react, motion, three, @react-three/fiber, @types/three, shadcn, tw-animate-css, all @fontsource packages.
- `components.json` and `src/hooks/use-mobile.ts` removed.
- Checks: tsc clean, vitest 35/35, vp check clean, vite build OK (JS 321 kB).
- Not committed or deployed yet at the time of writing.

## 19. Scope rule (from the user)

- Work only on the frontend UI (apps/panel/src). Do not change backend or system code (apps/api, services/agent, infrastructure/, install.sh, systemd units, helper ops, DB migrations) unless the user explicitly asks.

## 20. In-panel update fix

- The transient unit runs with a bare environment, so the Go build could not find its module cache. Fixed in `infrastructure/deployment/update.sh` (commit e85a45f): HOME, GOPATH, GOMODCACHE, GOCACHE and PATH are set before the installer runs.
- Verified on the VPS: the update unit finished with "== update finished OK"; zenex-api, zenex-helper and caddy are active; /login returns 200.
- Not verified through the panel button itself: the admin session had expired, so the unit was started directly with the same command.

## 7. Session 3: Frontend port React -> SolidJS (apps/panel)

- Ported in place (Solid conventions, same names): `src/App.tsx`, `src/main.tsx`, `src/layouts/AppLayout.tsx`, `src/components/app-sidebar.tsx`, `src/pages/{LoginPage,OverviewPage,WebsitesPage,SitePage}.tsx`, `src/app.test.tsx` (harness). ActivityPage/DomainsPage/ServerPage/SettingsPage needed no change.
- `App.tsx` wraps `<Routes>` in a keyed `<Show when={useLocation().pathname}>`: router `Routes` keeps the params of the first matched route (verified with a probe: plain Routes gave `id:none` after in-place navigation). Remove only if router.tsx is fixed.
- Hook args are not reactive (useJob, useActivity, useSiteLogs, useSystemUpdate...). Pages key JobProgress by job id. LogsTab mounts only when its tab is open, so it calls `useSiteLogs(id, true)`.
- Tests: `app.test.tsx` uses `@solidjs/testing-library`, `renderAt` dispatches popstate, typing uses `changeValue` (input + change). NOT YET PASSING: (a) component files still import react (FileManager.tsx etc.); (b) `@solidjs/testing-library` render loads a second Solid instance under Vitest, so Show/effects do not update. `solid-js/web` render works. Fix: add `/@solidjs\/testing-library/` to `test.server.deps.inline` in `apps/panel/vite.config.ts` (owner file).
- Pending: run `npx vitest run src/app.test.tsx` once the component port is done and (b) is fixed.

## 21. SolidJS frontend (latest)

- apps/panel is now SolidJS + Vite + Kobalte (headless primitives) + Tailwind. React, react-dom, @vitejs/plugin-react and the React testing library are removed.
- Data: src/api/query.ts uses Solid signals; hooks in src/api/queries.ts return getter objects (`q.data`, `q.isPending`); mutations via `m.mutate(vars, cbs)`.
- Router: src/lib/router.tsx (signal based); routes are `component: () => JSX` functions.
- UI: src/components/ui/* are plain Tailwind; dialog and tabs wrap Kobalte. Dialog children are resolved once (DialogBody) and DialogTrigger has no ARIA role.
- Pitfall found: a JSX prop passed as a getter (e.g. `actions={newWebsite()}`) is rebuilt each time it is read. Resolve such props with `children()` (see PageHeader).
- Tests: src/app.test.tsx mounts through solid-js/web directly. Vite config dedupes solid-js and inlines dependencies. 35/35 pass.
- Checks: vp check clean (0 warnings), tsc clean, vite build OK (JS about 189 kB).
- Not deployed to the VPS yet.

## 22. Frontend optimisation

- Every page is a lazy route chunk (Solid `lazy` + `Suspense`). First-load JS: 64 kB (23 kB gzip), down from 189 kB (59 kB gzip). Pages load on first visit.
- Checks: vp check clean, tsc clean, vitest 35/35, build OK.

## 23. QA pass on the live VPS (build fe5ff79)

- Deployed fe5ff79; services active; new bundle served.
- Headless Chrome, all routes (/, /websites, site page, /domains, /server, /activity, /settings, unknown route, /login): 0 exceptions, 0 console errors, 0 leaked undefined/NaN text, no unexpected API failures.
- Click-through (site tabs Logs/Files/Settings, WordPress login dialog, new-website dialog, Escape to close, notification bell, mobile drawer, no horizontal overflow on phone width, wrong-password error): 19/19 checks pass. The only 401 responses are the expected signed-out check and the wrong password.
- Not covered by this pass: creating/deleting a real site, DNS checks, the in-panel update button, visual review of the Solid build on phone and desktop.

## 24. Typography (latest)

- Headings: Bricolage Grotesque Variable. Body: Outfit Variable. Both self-hosted via @fontsource-variable (CSP-safe), with system fallbacks. Set in apps/panel/src/index.css.

## 25. Icon bug fix

- Icons shared one DOM shape. An icon used in two places (for example Layers in the sidebar and on the Overview card) lost its shape from the first place when it rendered in the second. Each render now copies its shape (apps/panel/src/components/icons.tsx). Regression test: icons.test.tsx (fails on the old code).

## 26. Site and server UI (latest)

- Site Overview: WordPress card (Open admin signs in without a password by posting the stored login to the site's wp-login.php in a new tab; username and password shown with show/hide and copy). Removed the old WordPress dialog from Settings; Settings keeps the danger zone.
- PHP version: the five newest releases (8.5 to 8.1). Versions the server does not have show "(not installed on this server)" and cannot be picked.
- Server page: new "Capacity and load" card (load 1/5/15 min, CPU cores, memory and disk totals, uptime) from existing metrics.
- Needs backend work (not done, backend is off-limits for now): file upload in the Files tab, installing the extra PHP versions, server details such as OS and IP.

- Open admin is a plain link to https://<domain>/wp-admin/, with the username and password shown on the card. Auto sign-in does NOT work from the browser: the panel and the site are different sites, so the browser drops WordPress's login cookie after the redirect (the server-side login itself works: 302 to wp-admin). A password-free sign-in needs a backend change (for example, a one-time login link served from the site's domain). Not done; needs approval.

## 27. Settings features (default PHP, WP auto-updates, backups, alerts, maintenance)

- API (apps/api): migration 0006 (sites.auto_update, sites.maintenance; the unused 0001 backups table is renamed to backups_legacy_0001; new backups table). Settings in system_settings: site_defaults, backup_settings, alert_settings (SMTP and Telegram secrets AES-GCM encrypted with a key from ZENEX_SECRET_KEY). Scheduler: WP updates 04:00, backups at schedule_hour (default 03:00), retention 7 days. Alerts from the monitor with threshold tracking, 30 min cooldown and recovery messages.
- Helper (services/agent): vhost.write maintenance flag (503 page); wp.update; backup.create and backup.delete (archives in /var/backups/zenex/<user>/). Operation timeout raised to 45 minutes; API helper client timeout to 50 minutes.
- Frontend (apps/panel): Settings cards for site defaults, backups and alerts; Overview toggles for maintenance mode and daily WordPress updates; Settings tab backup list with Back up now.
- Verified: go vet and tests (API and helper, helper also for Linux), frontend 40/40, lint, build. Migrations 0001-0006 applied to a fresh PostgreSQL 16 database.
- Not verified on the VPS yet. Alert delivery (SMTP and Telegram) needs real credentials to test.
