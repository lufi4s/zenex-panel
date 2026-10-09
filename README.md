# Zenex Panel

A self-hosted control panel for **WordPress websites** on your own Ubuntu server, in the style of Kinsta or EasyWP.
You bring the server. One command installs the panel, the web stack and a secure admin login. Then you add a domain,
create WordPress sites in a few clicks, back them up, restore them, and move sites over from cPanel.

> **Status.** The panel is under active development. The code is covered by automated tests (Go unit tests, a real
> PostgreSQL integration test, and UI tests). The most recent features (restore, restore from another panel, file upload,
> one-click WordPress admin, cPanel migration) have **not yet been run on a live server**. Try them on a test server first.
> See [Known limits](#known-limits).

---

## Contents

1. [What you get](#what-you-get)
2. [How it works](#how-it-works)
3. [Requirements](#requirements)
4. [Install](#install)
5. [First steps](#first-steps)
6. [Using the panel](#using-the-panel)
7. [Update](#update)
8. [Configuration](#configuration)
9. [Security model](#security-model)
10. [Development](#development)
11. [Repository layout](#repository-layout)
12. [Troubleshooting](#troubleshooting)
13. [Known limits](#known-limits)

---

## What you get

**Websites**
- Create a WordPress site on `name.yourdomain.com` in one dialog. The panel builds a separate Linux account, PHP-FPM pool,
  MariaDB database and user, web server config and HTTPS certificate, then installs WordPress. You watch each step live.
- Every site is isolated: its own user, files, PHP pool and database. A hacked site cannot read another site.
- Per site: restart PHP, take offline / bring online, maintenance page, daily WordPress core updates, change PHP version,
  access log, delete with typed confirmation.
- **Live status on the Websites list:** "Backing up · 45%", "Backup complete", "Restoring", "Migrating", "Building"…
- **Open admin** signs you in to the WordPress dashboard with one click (a one-time link that works once for 90 seconds).

**Files**
- Browse, edit text files, create files and folders, delete, and **upload** files (up to 64 MB each) from the browser.

**Domains and HTTPS**
- Add your domain, point a wildcard DNS record at the server, and the panel verifies it.
- HTTPS certificates (Let's Encrypt) are issued and renewed automatically by Caddy.

**Backups and restore**
- Hourly, daily or weekly schedule; keep 1–90 days.
- Store on this server or on a remote **SFTP** server (SSH key or password). Test the connection from the panel.
- **Back up all websites** with one button, with a progress bar per website.
- **Restore** any backup of a website. A copy of the current website is saved first, so a restore can be undone.
- **Restore on another panel:** connect a new panel to the same SFTP server and it lists every website that has a
  backup there. Pick one and restore it into a website — useful when you move to a new server.

**Migrate from cPanel**
- Sign in to a cPanel account over SSH. The panel finds the WordPress sites, copies the files and the database, builds
  the site here and fixes the addresses. It is **read-only on cPanel** and **never changes your DNS**. You switch the DNS
  yourself when the copy looks right.

**Operations**
- Server tiles and history charts (CPU load, memory, disk), service status, website uptime checks.
- Activity log of every action (who, what, result). Notifications bell.
- Alerts by email (SMTP) and Telegram when CPU, memory or disk cross a threshold, with recovery messages.
- Branding: panel name, colours, logo, favicon.
- One-click **panel update** from the Settings page.

---

## How it works

```
 Browser ──HTTPS──▶  zenex-api (Go, user "zenex")  ──────▶ PostgreSQL   (panel data, jobs, audit)
                          │
                          │  Unix socket, group "zenex"
                          ▼
                    zenex-helper (Go, root)  ──▶ fixed, validated operations only
                          │
          ┌───────────────┼──────────────────────────────────────────┐
          ▼               ▼                      ▼                   ▼
        Caddy        PHP-FPM pools       MariaDB (per-site DBs)   WP-CLI, tar, sftp, ssh
   (HTTPS, vhosts)  (one per site)
```

- **`zenex-api`** serves the web panel (a Solid single-page app embedded in the binary) and the JSON API. It never runs
  system commands itself.
- **`zenex-helper`** is a small root daemon on a Unix socket. It only performs a fixed list of operations (create a site
  account, write a PHP pool, restore a backup, …). Every argument is validated, every external program is an allowlisted
  absolute path called with an argument list (never a shell).
- **Jobs** are rows in PostgreSQL with ordered steps (so the UI can show progress). Builds are resumed after a restart;
  backups, restores and migrations that were running when the panel stopped are marked failed.

---

## Requirements

- A fresh **Ubuntu 24.04** server (root access). **2 GB RAM or more** is comfortable; on smaller servers the installer
  adds 2 GB of swap. The installer needs at least **5 GB free disk**; websites and backups need more.
- A public IP address and the ability to open ports **22, 80, 443** and the panel port (default **8443**).
- A domain name you control, to put websites on.

The installer sets up: PostgreSQL, MariaDB, PHP-FPM (+ common WordPress extensions), Redis, Caddy, Fail2Ban, UFW (firewall),
WP-CLI, Go (to build the panel from source).

---

## Install

On a fresh Ubuntu 24.04 server, as root:

```bash
curl -fsSL https://raw.githubusercontent.com/lufi4s/zenex-panel/main/infrastructure/deployment/install.sh | sudo bash
```

Optional settings (put them before `sudo bash`):

| Variable | Meaning | Default |
|---|---|---|
| `ZENEX_ADMIN_EMAIL` | Login email of the first administrator | `admin@zenex.local` |
| `ZENEX_PUBLIC_IP` | Server IP, if auto-detection is wrong | detected |
| `ZENEX_REF` | Branch or tag to install | `main` |

Example:

```bash
curl -fsSL https://raw.githubusercontent.com/lufi4s/zenex-panel/main/infrastructure/deployment/install.sh \
  | sudo ZENEX_ADMIN_EMAIL=you@example.com bash
```

The installer takes a few minutes. At the end it prints the panel address (for example `https://203.0.113.10:8443`).
The login and password are saved in `/root/zenex-admin-credentials.txt`.

- It is **safe to run again**: it keeps your data, your login and your secret key.
- It repairs common problems on its own (package manager locked, interrupted installs, flaky downloads, low memory,
  a port already in use, an expired certificate, a panel that does not start).
- The first certificate is **self-signed**, so the browser shows a warning for the panel address. Continue past it.

> ⚠️ **Back up `/etc/zenex/panel.env`.** It holds the secret key that website database and admin passwords are derived
> from. If you lose it, every website's database password changes and the sites stop working.

---

## First steps

1. Open the panel address and sign in with the credentials from `/root/zenex-admin-credentials.txt`.
2. **Domains → Add domain.** Enter `example.com`. At your DNS provider add the **wildcard** record the panel shows
   (`*` → your server IP). Press **Check DNS** until it says *DNS ready*.
3. **Websites → New website.** Choose a name (for example `shop`) and the domain. In a few minutes
   `https://shop.example.com` runs WordPress. Use **Open admin** on the website page to sign in to WordPress.
4. **Settings → Backups.** Choose how often, how long to keep backups and where (this server or SFTP). Save.
5. **Settings → Alerts.** Add an email (SMTP) or Telegram so you hear about problems.

---

## Using the panel

| Page | What it is for |
|---|---|
| **Overview** | Server health, your latest websites (with live status), service status |
| **Websites** | All sites, **New website**, **Back up all websites**, **Migrate from cPanel** (administrators) |
| **Website page** | *Overview* (status, actions, maintenance, updates, WordPress access, PHP version), *Files*, *Logs*, *Settings* (backups, restore, delete) |
| **Domains** | Add domains, check DNS, remove |
| **Server** | CPU / memory / disk history, services |
| **Activity** | Everything that happened, with search and filter |
| **Settings** (administrators) | Panel updates, site defaults, backups, backups on the backup server, alerts, branding |

### Restore a backup
Website page → **Settings** tab → **Backups** → **Restore** next to the backup you want. Confirm. The panel first saves a
copy of the current website (it appears in the list), then puts the files and database back.

### Restore on a new panel (move servers)
1. On the new panel, **Settings → Backups**: choose **Remote server (SFTP)** with the *same* server and folder as the old
   panel, and make sure the new panel's key (or password) can sign in. Save.
2. **Settings → Backups on the backup server** lists all websites found there. Create the website on the new panel first
   (same or a different domain), then press **Restore** on the backup and pick that website.
3. If the domain differs, addresses stored in the database are changed automatically.

### Migrate from cPanel
1. Add the website's domain under **Domains** (its DNS can stay on cPanel for now).
2. **Websites → Migrate from cPanel.** Enter the cPanel server, SSH port, username and password. SSH must be enabled on the
   account. The password is used for this migration only and is not saved.
3. Pick the WordPress site and press **Migrate**. Watch the steps: build the site, copy files and database, put them in
   place, check WordPress.
4. When it is done, check the site (for example with a `hosts` file entry on your computer). Then set the domain's DNS
   **A record** to the server IP shown. HTTPS is issued automatically once the DNS points here.
5. If a migration fails, **Try again** starts the copy again. Nothing on cPanel was changed.

---

## Update

- **From the panel:** Settings → Panel updates → **Update now**.
- **From the server:**

```bash
git -C /opt/zenex/src fetch -q --depth 1 origin main \
  && git -C /opt/zenex/src reset -q --hard FETCH_HEAD \
  && bash /opt/zenex/src/infrastructure/deployment/install.sh
```

Updates keep your data. Database migrations run automatically when the panel starts.

---

## Configuration

The panel reads `/etc/zenex/panel.env` (written by the installer). Main settings:

| Variable | Meaning |
|---|---|
| `ZENEX_ENV` | `production` or `development` (production requires TLS and a database) |
| `ZENEX_API_ADDR` | Listen address, for example `0.0.0.0:8443` |
| `ZENEX_DATABASE_URL` | PostgreSQL connection string |
| `ZENEX_TLS_CERT`, `ZENEX_TLS_KEY` | Certificate and key for the panel address |
| `ZENEX_SECRET_KEY` | Master secret. Derives site passwords and encrypts saved secrets. **Do not lose it.** |
| `ZENEX_HELPER_SOCKET` | Helper socket (default `/run/zenex/helper.sock`) |
| `ZENEX_PHP_VERSION` | PHP version for new sites (default `8.3`) |
| `ZENEX_PUBLIC_IP` | This server's public IP (shown in DNS instructions) |
| `ZENEX_NODE_NAME` | Name of this server in the panel |

Useful commands on the server:

```bash
systemctl status zenex-api zenex-helper caddy      # service state
journalctl -u zenex-api -n 100 --no-pager          # panel log
journalctl -u zenex-helper -n 100 --no-pager       # helper log
cat /var/log/zenex/install.log                     # installer log
```

---

## Security model

- **Passwords:** Argon2id. Sessions are random tokens stored only as SHA-256 hashes; the cookie is `HttpOnly`, `Secure`,
  `SameSite=Strict`. State-changing requests need a custom header (CSRF guard). An account locks for 15 minutes after
  5 failed sign-ins; sign-in is rate limited per IP.
- **No shell from untrusted input:** the root helper runs a fixed list of operations; inputs are validated; programs are
  allowlisted absolute paths with argument lists; fixed environment; timeouts and output caps.
- **Site isolation:** one Linux account, PHP-FPM pool, directory tree and database user per website. Uploaded PHP in
  `wp-content/uploads` is blocked, and dotfiles are not served.
- **Secrets:** website passwords are derived from the master key and are not stored. Saved SMTP, Telegram and SFTP
  secrets are encrypted (AES-GCM) with a key derived from the master key.
- **Audit log:** every sensitive action is recorded; the table rejects updates and deletes at the database level.
- **Browser hardening:** strict Content Security Policy, no framing, `nosniff`, no-store caching.
- **cPanel migration:** the cPanel password stays in memory, is passed to SSH only through an environment variable, and is
  removed from error messages. The migration only reads from cPanel. Loopback and link-local hosts are refused.

Read `docs/security.md` for what is implemented and what is still missing.

---

## Development

You need **Go 1.22+**, **Node 20.19+** (or 22+) and **PostgreSQL 16** (only for the database tests).

```bash
# Backend: API and helper
cd apps/api       && go vet ./... && go test ./...
cd services/agent && go vet ./... && go test ./...
GOOS=linux go vet ./...            # the helper targets Linux; some tests skip on Windows

# Database tests (optional, needs an empty database)
createdb zx_test
ZENEX_TEST_DATABASE_URL="postgres://localhost/zx_test?sslmode=disable" go test ./internal/store

# Front end
cd apps/panel && npm ci
npm test            # Vitest (jsdom)
npm run build       # type-check + build into apps/api/web/dist (embedded in the API binary)
npm run dev         # Vite dev server with a proxy to the API

# Run the API locally (from apps/api)
cd apps/api
export ZENEX_ENV=development ZENEX_DATABASE_URL=postgres://localhost/zenex?sslmode=disable
export ZENEX_SECRET_KEY=$(openssl rand -hex 24)
printf 'you@example.com\nA-long-password-123\n' | go run ./cmd/api create-admin
go run ./cmd/api serve
```

Without the helper (a Linux root service), site creation and other server operations are not available locally, but
sign-in, domains, settings and the UI work.

---

## Repository layout

```
apps/api/                 Go API + embedded UI
  cmd/api/                  main: serve, migrate, create-admin
  internal/                 httpapi, auth, store (PostgreSQL), jobs, provision, manage, monitor, alerts, schedule, …
  migrations/               SQL schema (applied automatically on start)
  web/dist/                 built front end (embedded)
apps/panel/               Front end: SolidJS + Vite + Tailwind + Kobalte
services/agent/           zenex-helper (root daemon) and its operations
packages/validation/      Shared input validation (site names, domains, accounts)
infrastructure/deployment/  install.sh, update.sh
docs/                     architecture.md, security.md
```

---

## Troubleshooting

- **Browser warns about the certificate:** the panel's own certificate is self-signed. Continue past the warning.
- **Cannot create a website — "DNS not pointing":** the exact address (`name.example.com`) must resolve to this server.
  Check the wildcard record under *Domains*.
- **"The server's helper service is not running":** `systemctl status zenex-helper`, then `journalctl -u zenex-helper`.
- **Installer stopped:** read the line that starts with `[zenex] STOPPED`; the full log is `/var/log/zenex/install.log`.
  Run the same command again.
- **Open admin does nothing:** allow pop-ups for the panel address. A website in maintenance mode cannot be opened this way.
- **cPanel scan fails:** SSH must be enabled on the cPanel account and the password must be correct; the host must be
  reachable from this server.

---

## Known limits

- Ubuntu 24.04 only. One server per panel.
- WordPress only.
- Websites are created as `name.yourdomain.com`. A website on the bare domain is possible only through cPanel migration.
- No password reset by email, no two-factor sign-in, no list of active sessions yet.
- The malware scanner (ClamAV / YARA) and the separate security engine described in early plans are **not built**.
- Backups, restores and migrations run inside the panel process; if the panel restarts they stop and are marked failed
  (start them again).
- The cPanel migration needs SSH access with a password and cannot yet import a cPanel backup file.
- Restore from another panel needs the target website to exist already, and only lists backups made after manifests were
  introduced.
- Features added most recently have only been tested with automated tests, not yet on a live server.
