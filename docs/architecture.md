# Zenex Architecture

## Components

| Component | Language | Location | Status |
|---|---|---|---|
| Panel (customer UI) | SvelteKit / TypeScript | `apps/panel` | not started |
| Admin UI | SvelteKit / TypeScript | `apps/admin` | not started |
| API + UI | Go + embedded HTML/JS | `apps/api` | login, sessions, RBAC, audit, live host metrics, embedded panel UI |
| Zenex Agent | Go (Linux) | `services/agent` | executor + Nginx actions |
| Security Engine | Go | `services/security-engine` | not started |
| Migration Engine | Go | `services/migration-engine` | not started |
| Provisioner | Go | `services/provisioner` | not started |
| Shared validation | Go | `packages/validation` | done |
| Central DB schema | PostgreSQL | `apps/api/migrations` (embedded, applied on start) | 0001 applied and verified |

## Trust boundaries

1. **Browser <-> API**: session cookie, CSRF token, RBAC on every route.
2. **API <-> Agent**: outbound HTTPS from the Agent to the API, mutual TLS with a
   per-node client certificate. The API never opens inbound connections to the VPS.
3. **Agent <-> OS**: the Agent runs as root only for its privileged helper; every
   external program is an allowlisted absolute path, invoked with an argv array.
4. **Site <-> site**: each WordPress site has its own Linux UID/GID, PHP-FPM pool,
   socket, filesystem tree, and MariaDB user.

## Job model

Long-running work is a row in `jobs` with ordered `job_steps`. The state machine in
`apps/api/internal/jobs` governs transitions. Steps are idempotent and recorded so a
restarted worker resumes at the first non-succeeded step.

## Ordering rule for web-server changes

`nginx -t` must pass before any `systemctl reload nginx`. This is implemented in
`services/agent/internal/nginx.Reload` and is not bypassable through the Agent API.
