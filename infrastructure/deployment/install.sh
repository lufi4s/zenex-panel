#!/usr/bin/env bash
# Zenex all-in-one installer for a fresh Ubuntu 24.04 LTS VPS.
#
# One command installs and starts the panel, then prints the access link:
#   https://<server-ip>:8443
#
# It installs and configures:
#   - PostgreSQL (panel database), Go toolchain, git
#   - Zenex API + web UI, built from source and run as a hardened systemd service
#   - self-signed TLS certificate for the server IP
#   - the base web stack for hosted sites: nginx, PHP-FPM, MariaDB, Redis
#   - UFW firewall (default deny; allows 22, 80, 443, 8443)
#   - an administrator account with a generated password
#
# Usage (as root):
#   curl -fsSL https://raw.githubusercontent.com/lufi4s/zenex-panel/main/infrastructure/deployment/install.sh | sudo bash
#
# Optional environment overrides:
#   ZENEX_ADMIN_EMAIL   admin login email         (default admin@zenex.local)
#   ZENEX_PUBLIC_IP     IP to put in the TLS cert  (default: detected)
#   ZENEX_REPO_URL      git repository to build    (default: lufi4s/zenex-panel)
#   ZENEX_REF           branch or tag to build     (default: main)
#
# Safe to re-run: existing secrets and the admin account are kept.

set -euo pipefail

readonly ZENEX_USER="zenex"
readonly CONF_DIR="/etc/zenex"
readonly ENV_FILE="/etc/zenex/panel.env"
readonly TLS_DIR="/etc/zenex/tls"
readonly STATE_DIR="/var/lib/zenex"
readonly LOG_DIR="/var/log/zenex"
readonly SRC_DIR="/opt/zenex/src"
readonly BIN_PATH="/usr/local/bin/zenex-api"
readonly UNIT_PATH="/etc/systemd/system/zenex-api.service"
readonly CREDS_FILE="/root/zenex-admin-credentials.txt"
readonly PANEL_PORT="8443"
readonly DB_NAME="zenex_panel"
readonly DB_ROLE="zenex_panel"

ZENEX_ADMIN_EMAIL="${ZENEX_ADMIN_EMAIL:-admin@zenex.local}"
ZENEX_REPO_URL="${ZENEX_REPO_URL:-https://github.com/lufi4s/zenex-panel.git}"
ZENEX_REF="${ZENEX_REF:-main}"

log()  { printf '\n[zenex] %s\n' "$*"; }
fail() { printf '\n[zenex] ERROR: %s\n' "$*" >&2; exit 1; }

require_root() {
    [[ "$(id -u)" -eq 0 ]] || fail "run as root (use sudo)"
}

require_ubuntu_2404() {
    [[ -r /etc/os-release ]] || fail "cannot read /etc/os-release"
    # shellcheck disable=SC1091
    . /etc/os-release
    [[ "${ID:-}" == "ubuntu" && "${VERSION_ID:-}" == "24.04" ]] \
        || fail "Ubuntu 24.04 LTS is required; found ${PRETTY_NAME:-unknown}"
}

install_packages() {
    log "installing system packages (this takes a few minutes)"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq --no-install-recommends \
        ca-certificates curl git openssl \
        golang-go \
        postgresql \
        nginx \
        php-fpm php-mysql php-curl php-gd php-mbstring php-xml php-zip php-intl php-redis \
        mariadb-server \
        redis-server \
        ufw \
        fail2ban \
        certbot python3-certbot-nginx
}

create_service_user() {
    if ! id -u "$ZENEX_USER" >/dev/null 2>&1; then
        useradd --system --home-dir "$STATE_DIR" --shell /usr/sbin/nologin "$ZENEX_USER"
    fi
}

create_dirs() {
    log "creating Zenex directories"
    install -d -m 0750 -o root -g root "$CONF_DIR"
    install -d -m 0750 -o root -g "$ZENEX_USER" "$TLS_DIR"
    install -d -m 0750 -o "$ZENEX_USER" -g "$ZENEX_USER" "$STATE_DIR"
    install -d -m 0750 -o "$ZENEX_USER" -g adm "$LOG_DIR"
    install -d -m 0755 -o root -g root "$(dirname "$SRC_DIR")"
}

fetch_source() {
    log "fetching source ($ZENEX_REPO_URL @ $ZENEX_REF)"
    if [[ -d "$SRC_DIR/.git" ]]; then
        git -C "$SRC_DIR" fetch --depth 1 origin "$ZENEX_REF"
        git -C "$SRC_DIR" reset --hard FETCH_HEAD
    else
        rm -rf "$SRC_DIR"
        git clone --depth 1 --branch "$ZENEX_REF" "$ZENEX_REPO_URL" "$SRC_DIR"
    fi
}

build_api() {
    log "building Zenex API"
    (
        cd "$SRC_DIR/apps/api"
        GOFLAGS=-mod=readonly go build -buildvcs=false -trimpath -o "$BIN_PATH.new" ./cmd/api
    )
    install -m 0755 -o root -g root "$BIN_PATH.new" "$BIN_PATH"
    rm -f "$BIN_PATH.new"
}

detect_public_ip() {
    if [[ -n "${ZENEX_PUBLIC_IP:-}" ]]; then
        printf '%s' "$ZENEX_PUBLIC_IP"
        return
    fi
    ip -4 route get 1.1.1.1 | awk '{for (i = 1; i <= NF; i++) if ($i == "src") { print $(i + 1); exit }}'
}

# Loads an existing secret from panel.env, or generates a new one.
env_value() {
    local key="$1" fallback="$2"
    if [[ -f "$ENV_FILE" ]] && grep -q "^${key}=" "$ENV_FILE"; then
        grep "^${key}=" "$ENV_FILE" | head -n1 | cut -d= -f2-
    else
        printf '%s' "$fallback"
    fi
}

setup_database() {
    log "configuring PostgreSQL"
    systemctl enable --now postgresql

    local db_password
    db_password="$(env_value ZENEX_DB_PASSWORD "$(openssl rand -hex 24)")"

    # Role and database are created once; re-runs only ensure they exist.
    sudo -u postgres psql -v ON_ERROR_STOP=1 -q <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${DB_ROLE}') THEN
    CREATE ROLE ${DB_ROLE} LOGIN PASSWORD '${db_password}';
  ELSE
    ALTER ROLE ${DB_ROLE} LOGIN PASSWORD '${db_password}';
  END IF;
END
\$\$;
SELECT 'CREATE DATABASE ${DB_NAME} OWNER ${DB_ROLE}'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${DB_NAME}')\gexec
SQL
    DB_PASSWORD_RESULT="$db_password"
}

write_env_file() {
    log "writing service configuration"
    local public_ip="$1" db_password="$2"

    cat > "$ENV_FILE.tmp" <<EOF
ZENEX_ENV=production
ZENEX_API_ADDR=0.0.0.0:${PANEL_PORT}
ZENEX_DATABASE_URL=postgres://${DB_ROLE}:${db_password}@127.0.0.1:5432/${DB_NAME}?sslmode=disable
ZENEX_TLS_CERT=${TLS_DIR}/panel.crt
ZENEX_TLS_KEY=${TLS_DIR}/panel.key
ZENEX_DB_PASSWORD=${db_password}
ZENEX_PUBLIC_IP=${public_ip}
EOF
    chown root:"$ZENEX_USER" "$ENV_FILE.tmp"
    chmod 0640 "$ENV_FILE.tmp"
    mv -f "$ENV_FILE.tmp" "$ENV_FILE"
}

create_tls_cert() {
    local public_ip="$1"
    if [[ -f "$TLS_DIR/panel.crt" && -f "$TLS_DIR/panel.key" ]]; then
        log "keeping existing TLS certificate"
    else
        log "creating self-signed TLS certificate for $public_ip"
        openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
            -days 3650 -subj "/CN=zenex-panel" \
            -addext "subjectAltName=IP:${public_ip},IP:127.0.0.1" \
            -keyout "$TLS_DIR/panel.key" -out "$TLS_DIR/panel.crt" 2>/dev/null
    fi
    chown root:"$ZENEX_USER" "$TLS_DIR/panel.key" "$TLS_DIR/panel.crt"
    chmod 0640 "$TLS_DIR/panel.key"
    chmod 0644 "$TLS_DIR/panel.crt"
}

install_unit() {
    log "installing systemd service"
    cat > "$UNIT_PATH" <<EOF
[Unit]
Description=Zenex Panel API
After=network-online.target postgresql.service
Wants=network-online.target postgresql.service

[Service]
Type=simple
User=${ZENEX_USER}
Group=${ZENEX_USER}
EnvironmentFile=${ENV_FILE}
ExecStart=${BIN_PATH} serve
WorkingDirectory=${STATE_DIR}
Restart=on-failure
RestartSec=3
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ProtectHome=true
ProtectKernelTunables=true
ProtectKernelModules=true
ProtectControlGroups=true
RestrictSUIDSGID=true
LockPersonality=true
ReadWritePaths=${STATE_DIR} ${LOG_DIR}
ReadOnlyPaths=${TLS_DIR}
CapabilityBoundingSet=
AmbientCapabilities=
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
}

create_admin_once() {
    if [[ -f "$CREDS_FILE" ]]; then
        log "administrator already exists; keeping existing account"
        return
    fi
    log "creating administrator account"
    local admin_password
    admin_password="$(openssl rand -hex 12)"
    (
        set -a
        # shellcheck disable=SC1090
        . "$ENV_FILE"
        set +a
        printf '%s\n%s\n' "$ZENEX_ADMIN_EMAIL" "$admin_password" \
            | "$BIN_PATH" create-admin >/dev/null
    )
    umask 077
    cat > "$CREDS_FILE" <<EOF
Zenex panel administrator
URL:      https://$(detect_public_ip_cached):${PANEL_PORT}
Email:    ${ZENEX_ADMIN_EMAIL}
Password: ${admin_password}

Change this password after first sign-in. This file is readable only by root.
EOF
    chmod 0600 "$CREDS_FILE"
    ADMIN_PASSWORD_NEW="$admin_password"
}

configure_firewall() {
    log "configuring firewall (default deny; allow 22, 80, 443, ${PANEL_PORT})"
    ufw --force default deny incoming
    ufw --force default allow outgoing
    ufw allow 22/tcp comment 'ssh'
    ufw allow 80/tcp comment 'http'
    ufw allow 443/tcp comment 'https'
    ufw allow "${PANEL_PORT}/tcp" comment 'zenex panel'
    ufw --force enable
}

enable_services() {
    log "starting services"
    systemctl enable --now nginx
    systemctl enable --now mariadb
    systemctl enable --now redis-server
    systemctl enable --now fail2ban
    systemctl enable --now "php$(php -r 'echo PHP_MAJOR_VERSION.".".PHP_MINOR_VERSION;')-fpm"
    systemctl enable --now zenex-api
    systemctl restart zenex-api
}

wait_for_panel() {
    log "waiting for the panel to answer"
    local i
    for i in $(seq 1 30); do
        if curl -fsk --max-time 3 "https://127.0.0.1:${PANEL_PORT}/api/v1/health" >/dev/null 2>&1; then
            return 0
        fi
        sleep 2
    done
    systemctl --no-pager status zenex-api || true
    journalctl -u zenex-api --no-pager -n 40 || true
    fail "panel did not start; see the logs above"
}

detect_public_ip_cached() {
    if [[ -z "${PUBLIC_IP_CACHE:-}" ]]; then
        PUBLIC_IP_CACHE="$(detect_public_ip)"
    fi
    printf '%s' "$PUBLIC_IP_CACHE"
}

print_summary() {
    local ip
    ip="$(detect_public_ip_cached)"
    cat <<EOF

============================================================
 Zenex panel is installed and running.

 Open this in your browser:
     https://${ip}:${PANEL_PORT}

 Your browser will warn about the certificate because it is
 self-signed. This is expected; accept it to continue.

 Sign in with:
     Email:    ${ZENEX_ADMIN_EMAIL}
     Password: ${ADMIN_PASSWORD_NEW:-(unchanged; see ${CREDS_FILE})}

 Credentials are saved in ${CREDS_FILE} (root only).
 Re-running this script keeps your existing data.

 Not yet available: the VPS Agent, site creation, and WordPress
 provisioning. The dashboard shows live server metrics only.
============================================================
EOF
}

main() {
    require_root
    require_ubuntu_2404
    install_packages
    create_service_user
    create_dirs
    fetch_source
    build_api

    local public_ip
    public_ip="$(detect_public_ip_cached)"
    [[ -n "$public_ip" ]] || fail "could not detect the server IP; set ZENEX_PUBLIC_IP and re-run"

    setup_database
    write_env_file "$public_ip" "$DB_PASSWORD_RESULT"
    create_tls_cert "$public_ip"
    install_unit
    configure_firewall
    enable_services
    wait_for_panel
    create_admin_once
    nginx -t
    print_summary
}

main "$@"
