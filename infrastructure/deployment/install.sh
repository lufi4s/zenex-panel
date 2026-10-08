#!/usr/bin/env bash
# ============================================================================
#  Zenex Panel - one-command installer for a fresh Ubuntu 24.04 server
# ============================================================================
#  Run this single command as root (or with sudo):
#
#    curl -fsSL https://raw.githubusercontent.com/lufi4s/zenex-panel/main/infrastructure/deployment/install.sh | sudo bash
#
#  At the end it prints the address to open in your browser, plus the login.
#
#  The script repairs common problems on its own:
#    - Ubuntu's background update holding the package manager (waits for it)
#    - interrupted package installs (finishes them)
#    - flaky network downloads (retries)
#    - small servers running out of memory during build (adds swap)
#    - wrong file permissions (fixes them)
#    - a port already in use (picks the next free port)
#    - an out-of-date or mismatched certificate (creates a new one)
#    - a panel that does not start (reads the logs and repairs, then retries)
#  Re-running the same command is always safe. It keeps your data and login.
#
#  Optional settings (put before "sudo bash"):
#    ZENEX_ADMIN_EMAIL=you@example.com   login email (default admin@zenex.local)
#    ZENEX_PUBLIC_IP=1.2.3.4             server IP, if auto-detection is wrong
#    ZENEX_REF=main                      branch or tag to install
# ============================================================================

set -Eeuo pipefail

readonly ZENEX_USER="zenex"
readonly CONF_DIR="/etc/zenex"
readonly ENV_FILE="/etc/zenex/panel.env"
readonly TLS_DIR="/etc/zenex/tls"
readonly STATE_DIR="/var/lib/zenex"
readonly LOG_DIR="/var/log/zenex"
readonly LOG_FILE="/var/log/zenex/install.log"
readonly OPT_DIR="/opt/zenex"
readonly SRC_DIR="/opt/zenex/src"
readonly BIN_PATH="/usr/local/bin/zenex-api"
readonly UNIT_PATH="/etc/systemd/system/zenex-api.service"
readonly CREDS_FILE="/root/zenex-admin-credentials.txt"
readonly DB_NAME="zenex_panel"
readonly DB_ROLE="zenex_panel"
readonly PORT_CANDIDATES="8443 8444 8445 8446 8447 8448 8449 8450"
readonly RUN_DIR="/run/zenex"
readonly HELPER_BIN="/usr/local/sbin/zenex-helper"
readonly HELPER_SOCKET="/run/zenex/helper.sock"
readonly HELPER_UNIT="/etc/systemd/system/zenex-helper.service"
readonly WP_BIN="/usr/local/bin/wp"
readonly WP_URL="https://raw.githubusercontent.com/wp-cli/builds/gh-pages/phar/wp-cli.phar"

ZENEX_ADMIN_EMAIL="${ZENEX_ADMIN_EMAIL:-admin@zenex.local}"
ZENEX_REPO_URL="${ZENEX_REPO_URL:-https://github.com/lufi4s/zenex-panel.git}"
ZENEX_REF="${ZENEX_REF:-main}"

readonly STEP_TOTAL=15
STEP_NO=0
CURRENT_STEP="starting"
PANEL_PORT=""
PUBLIC_IP=""
DB_PASSWORD=""
PHP_VER=""
SECRET_KEY=""
ADMIN_PASSWORD_NEW=""

# ---------------------------------------------------------------------------
# Output helpers
# ---------------------------------------------------------------------------

step() {
    STEP_NO=$((STEP_NO + 1))
    CURRENT_STEP="$1"
    printf '\n[%d/%d] %s\n' "$STEP_NO" "$STEP_TOTAL" "$1"
}

info() { printf '      %s\n' "$*"; }

fail() {
    printf '\n[zenex] STOPPED: %s\n' "$*"
    printf '[zenex] Full log: %s\n' "$LOG_FILE"
    exit 1
}

on_error() {
    local code=$? line=$1
    printf '\n============================================================\n'
    printf ' The installer could not finish this step: %s\n' "$CURRENT_STEP"
    printf ' Technical detail: failed at script line %s (exit code %s).\n' "$line" "$code"
    printf ' Nothing you have set up is lost. Run the same command again.\n'
    printf ' If it keeps failing, send this file to support:\n'
    printf '     %s\n' "$LOG_FILE"
    printf '============================================================\n'
    exit "$code"
}
trap 'on_error $LINENO' ERR

# retry <attempts> <delay-seconds> <command...>
retry() {
    local attempts=$1 delay=$2 n=1
    shift 2
    until "$@"; do
        if (( n >= attempts )); then
            return 1
        fi
        info "that did not work (try $n of $attempts). Trying again in ${delay}s..."
        sleep "$delay"
        n=$((n + 1))
    done
}

# ---------------------------------------------------------------------------
# Checks and self-repair
# ---------------------------------------------------------------------------

require_root() {
    if [[ "$(id -u)" -ne 0 ]]; then
        printf '[zenex] Please run this with sudo:\n'
        printf '        curl -fsSL <link> | sudo bash\n'
        exit 1
    fi
}

check_os() {
    [[ -r /etc/os-release ]] || fail "this does not look like Ubuntu (no /etc/os-release)."
    # shellcheck disable=SC1091
    . /etc/os-release
    if [[ "${ID:-}" != "ubuntu" || "${VERSION_ID:-}" != "24.04" ]]; then
        fail "this installer needs Ubuntu 24.04. This server runs ${PRETTY_NAME:-an unknown system}."
    fi
    info "Ubuntu 24.04 detected"
}

check_disk() {
    local free_gb
    free_gb="$(df -BG --output=avail / | tail -n1 | tr -dc '0-9')"
    if (( free_gb < 5 )); then
        fail "not enough free disk space (${free_gb} GB free, 5 GB needed). Free up space or use a larger server."
    fi
    info "disk space OK (${free_gb} GB free)"
}

check_internet() {
    if retry 3 5 curl -fsS --max-time 10 -o /dev/null https://github.com; then
        info "internet connection OK"
    else
        fail "this server cannot reach the internet (github.com). Check the server's network and try again."
    fi
}

# Busy means the real package-manager locks are held. Checking process names
# is not reliable: Ubuntu keeps an idle unattended-upgrade process running.
package_manager_busy() {
    if flock -n /var/lib/dpkg/lock-frontend -c true \
        && flock -n /var/lib/apt/lists/lock -c true; then
        return 1
    fi
    return 0
}

wait_for_package_manager() {
    local i
    for i in $(seq 1 60); do
        if ! package_manager_busy; then
            return 0
        fi
        info "Ubuntu is running its own update (normal on new servers). Waiting..."
        sleep 10
    done
    info "the update is still running after 10 minutes; continuing anyway."
}

ensure_swap() {
    local mem_mb swap_mb
    mem_mb="$(awk '/MemTotal/ {print int($2 / 1024)}' /proc/meminfo)"
    swap_mb="$(awk '/SwapTotal/ {print int($2 / 1024)}' /proc/meminfo)"
    if (( mem_mb >= 2048 || swap_mb >= 1024 )); then
        info "memory OK (${mem_mb} MB RAM, ${swap_mb} MB swap)"
        return 0
    fi
    info "small server detected (${mem_mb} MB RAM). Adding 2 GB swap so the build does not run out of memory."
    if [[ ! -f /swapfile ]]; then
        fallocate -l 2G /swapfile 2>/dev/null || dd if=/dev/zero of=/swapfile bs=1M count=2048 status=none
        chmod 600 /swapfile
        mkswap /swapfile >/dev/null
    fi
    swapon /swapfile 2>/dev/null || true
    grep -q '^/swapfile ' /etc/fstab || printf '/swapfile none swap sw 0 0\n' >> /etc/fstab
}

install_packages() {
    export DEBIAN_FRONTEND=noninteractive
    wait_for_package_manager
    if ! dpkg --configure -a >/dev/null 2>&1; then
        info "repairing an interrupted package install"
        dpkg --configure -a || true
    fi
    retry 3 15 apt-get update -qq
    retry 3 20 apt-get install -y -qq --no-install-recommends \
        -o Dpkg::Options::=--force-confdef -o Dpkg::Options::=--force-confold \
        ca-certificates curl git openssl \
        golang-go \
        postgresql \
        nginx \
        php-cli php-fpm php-mysql php-curl php-gd php-mbstring php-xml php-zip php-intl php-redis \
        mariadb-server \
        redis-server \
        ufw \
        fail2ban \
        certbot python3-certbot-nginx
    info "all system software installed"
}

create_service_user() {
    if ! id -u "$ZENEX_USER" >/dev/null 2>&1; then
        useradd --system --home-dir "$STATE_DIR" --shell /usr/sbin/nologin "$ZENEX_USER"
    fi
}

prepare_directories() {
    # The service user must be able to pass through CONF_DIR to reach tls/.
    # 0710 = owner full, group traverse-only (no listing), others nothing.
    install -d -m 0710 -o root -g "$ZENEX_USER" "$CONF_DIR"
    install -d -m 0750 -o root -g "$ZENEX_USER" "$TLS_DIR"
    install -d -m 0750 -o "$ZENEX_USER" -g "$ZENEX_USER" "$STATE_DIR"
    install -d -m 0755 -o root -g adm "$LOG_DIR"
    install -d -m 0755 -o root -g root "$OPT_DIR"
    fix_file_permissions
    info "folders ready"
}

# Applied on every run and after every repair, so permissions always match.
# Every folder on the path to a file must be traversable by the service user,
# not only the file itself. Missing this caused "permission denied" on panel.crt.
fix_file_permissions() {
    if [[ -d "$CONF_DIR" ]]; then
        chown root:"$ZENEX_USER" "$CONF_DIR"
        chmod 0710 "$CONF_DIR"
    fi
    if [[ -d "$TLS_DIR" ]]; then
        chown root:"$ZENEX_USER" "$TLS_DIR"
        chmod 0750 "$TLS_DIR"
    fi
    if [[ -f "$TLS_DIR/panel.key" ]]; then
        chown root:"$ZENEX_USER" "$TLS_DIR/panel.key"
        chmod 0640 "$TLS_DIR/panel.key"
    fi
    if [[ -f "$TLS_DIR/panel.crt" ]]; then
        chown root:"$ZENEX_USER" "$TLS_DIR/panel.crt"
        chmod 0644 "$TLS_DIR/panel.crt"
    fi
    if [[ -f "$ENV_FILE" ]]; then
        # systemd reads this file as root, so the service user does not need it.
        chown root:root "$ENV_FILE"
        chmod 0600 "$ENV_FILE"
    fi
    if [[ -d "$STATE_DIR" ]]; then
        chown -R "$ZENEX_USER":"$ZENEX_USER" "$STATE_DIR"
    fi
}

fetch_source() {
    if [[ -d "$SRC_DIR/.git" ]] \
        && git -C "$SRC_DIR" fetch -q --depth 1 origin "$ZENEX_REF" \
        && git -C "$SRC_DIR" reset -q --hard FETCH_HEAD; then
        info "updated to the latest version ($ZENEX_REF)"
        return 0
    fi
    info "downloading a fresh copy of the panel"
    rm -rf "$SRC_DIR"
    retry 3 10 git clone -q --depth 1 --branch "$ZENEX_REF" "$ZENEX_REPO_URL" "$SRC_DIR"
    info "download complete"
}

build_helper() {
    local attempt
    for attempt in 1 2; do
        if (cd "$SRC_DIR/services/agent" && go build -buildvcs=false -trimpath -o "$HELPER_BIN.new" ./cmd/zenex-helper); then
            install -m 0755 -o root -g root "$HELPER_BIN.new" "$HELPER_BIN"
            rm -f "$HELPER_BIN.new"
            info "website helper built"
            return 0
        fi
        info "helper build failed (attempt $attempt of 2). Clearing the build cache and trying again..."
        go clean -cache >/dev/null 2>&1 || true
        sleep 5
    done
    return 1
}

# WP-CLI is the official WordPress command-line tool. It is installed only after
# its published SHA-512 checksum matches the download.
install_wp_cli() {
    if [[ -f "$WP_BIN" ]] && php "$WP_BIN" --info >/dev/null 2>&1; then
        info "WP-CLI already installed"
        return 0
    fi
    local tmp sum
    tmp="$(mktemp)"
    sum="$(mktemp)"
    retry 3 5 curl -fsSL -o "$tmp" "$WP_URL"
    retry 3 5 curl -fsSL -o "$sum" "$WP_URL.sha512"
    local expected actual
    expected="$(awk '{print $1}' "$sum")"
    actual="$(sha512sum "$tmp" | awk '{print $1}')"
    if [[ -z "$expected" || "$expected" != "$actual" ]]; then
        rm -f "$tmp" "$sum"
        fail "the WP-CLI download did not match its checksum. Re-run the command in a few minutes."
    fi
    install -m 0755 -o root -g root "$tmp" "$WP_BIN"
    rm -f "$tmp" "$sum"
    info "WP-CLI installed"
}

install_helper_unit() {
    cat > "$HELPER_UNIT" <<EOF
[Unit]
Description=Zenex website helper (runs fixed site-building operations only)
After=network-online.target
Wants=network-online.target
StartLimitIntervalSec=0

[Service]
Type=simple
User=root
Environment=ZENEX_HELPER_SOCKET=${HELPER_SOCKET}
Environment=ZENEX_HELPER_GROUP=${ZENEX_USER}
ExecStart=${HELPER_BIN}
Restart=always
RestartSec=3
PrivateTmp=true

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
}

start_helper() {
    systemctl enable zenex-helper >/dev/null 2>&1 || true
    systemctl restart zenex-helper
    local i
    for i in $(seq 1 20); do
        if [[ -S "$HELPER_SOCKET" ]]; then
            info "website helper running"
            return 0
        fi
        sleep 1
    done
    fail "the website helper did not start. Run: journalctl -u zenex-helper -n 50"
}

build_api() {
    local attempt
    for attempt in 1 2; do
        if (cd "$SRC_DIR/apps/api" && go build -buildvcs=false -trimpath -o "$BIN_PATH.new" ./cmd/api); then
            install -m 0755 -o root -g root "$BIN_PATH.new" "$BIN_PATH"
            rm -f "$BIN_PATH.new"
            info "panel program built"
            return 0
        fi
        info "the build failed (attempt $attempt of 2). Clearing the build cache and trying again..."
        go clean -cache -modcache >/dev/null 2>&1 || true
        sleep 5
    done
    return 1
}

# ---------------------------------------------------------------------------
# Network: IP and port
# ---------------------------------------------------------------------------

is_ipv4() {
    [[ "$1" =~ ^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$ ]]
}

detect_public_ip() {
    local ip=""
    if [[ -n "${ZENEX_PUBLIC_IP:-}" ]]; then
        ip="$ZENEX_PUBLIC_IP"
    else
        ip="$(curl -4fsS --max-time 8 https://api.ipify.org 2>/dev/null || true)"
        if ! is_ipv4 "$ip"; then
            ip="$(ip -4 route get 1.1.1.1 2>/dev/null \
                | awk '{for (i = 1; i <= NF; i++) if ($i == "src") { print $(i + 1); exit }}' || true)"
        fi
    fi
    is_ipv4 "$ip" || fail "could not detect this server's IP address. Re-run with your server IP: ZENEX_PUBLIC_IP=<ip> ... | sudo bash"
    PUBLIC_IP="$ip"
    info "server IP: $PUBLIC_IP"
}

port_listening() {
    ss -tln 2>/dev/null | awk '{print $4}' | grep -q ":$1\$"
}

port_owned_by_zenex() {
    ss -tlnp 2>/dev/null | grep ":$1 " | grep -q 'zenex-api'
}

choose_panel_port() {
    local p
    for p in $PORT_CANDIDATES; do
        if port_listening "$p" && ! port_owned_by_zenex "$p"; then
            continue
        fi
        PANEL_PORT="$p"
        info "panel port: $PANEL_PORT"
        return 0
    done
    fail "ports 8443 to 8450 are all in use by other programs. Stop one of them and re-run."
}

# ---------------------------------------------------------------------------
# Database and configuration
# ---------------------------------------------------------------------------

existing_env_value() {
    if [[ -f "$ENV_FILE" ]]; then
        grep "^$1=" "$ENV_FILE" | head -n1 | cut -d= -f2- || true
    fi
}

wait_for_postgres() {
    retry 30 2 sudo -u postgres pg_isready -q
}

setup_database() {
    systemctl enable --now postgresql >/dev/null 2>&1 || true
    wait_for_postgres || fail "PostgreSQL did not start. Run: systemctl status postgresql"

    if [[ -z "$DB_PASSWORD" ]]; then
        DB_PASSWORD="$(existing_env_value ZENEX_DB_PASSWORD)"
    fi
    if [[ -z "$DB_PASSWORD" ]]; then
        DB_PASSWORD="$(openssl rand -hex 24)"
    fi

    # Hex-only password: safe to embed in SQL without escaping.
    sudo -u postgres psql -v ON_ERROR_STOP=1 -q <<SQL
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = '${DB_ROLE}') THEN
    CREATE ROLE ${DB_ROLE} LOGIN PASSWORD '${DB_PASSWORD}';
  ELSE
    ALTER ROLE ${DB_ROLE} LOGIN PASSWORD '${DB_PASSWORD}';
  END IF;
END
\$\$;
SELECT 'CREATE DATABASE ${DB_NAME} OWNER ${DB_ROLE}'
  WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = '${DB_NAME}')\gexec
SQL
    info "database ready"
}

write_env_file() {
    local tmp="$ENV_FILE.tmp" php_ver
    # The server secret derives every site password. It must never change once
    # sites exist, so it is generated once and then kept.
    if [[ -z "$SECRET_KEY" ]]; then
        SECRET_KEY="$(existing_env_value ZENEX_SECRET_KEY)"
    fi
    if [[ -z "$SECRET_KEY" ]]; then
        SECRET_KEY="$(openssl rand -hex 32)"
    fi
    php_ver="$(php -r 'echo PHP_MAJOR_VERSION, ".", PHP_MINOR_VERSION;')"

    cat > "$tmp" <<EOF
ZENEX_ENV=production
ZENEX_API_ADDR=0.0.0.0:${PANEL_PORT}
ZENEX_DATABASE_URL=postgres://${DB_ROLE}:${DB_PASSWORD}@127.0.0.1:5432/${DB_NAME}?sslmode=disable
ZENEX_TLS_CERT=${TLS_DIR}/panel.crt
ZENEX_TLS_KEY=${TLS_DIR}/panel.key
ZENEX_DB_PASSWORD=${DB_PASSWORD}
ZENEX_PUBLIC_IP=${PUBLIC_IP}
ZENEX_SECRET_KEY=${SECRET_KEY}
ZENEX_PHP_VERSION=${php_ver}
ZENEX_HELPER_SOCKET=${HELPER_SOCKET}
ZENEX_NODE_NAME=this-server
EOF
    mv -f "$tmp" "$ENV_FILE"
    fix_file_permissions
    info "settings saved"
}

cert_is_valid() {
    [[ -f "$TLS_DIR/panel.crt" && -f "$TLS_DIR/panel.key" ]] || return 1
    openssl x509 -in "$TLS_DIR/panel.crt" -noout -checkend 604800 >/dev/null 2>&1 || return 1
    openssl x509 -in "$TLS_DIR/panel.crt" -noout -ext subjectAltName 2>/dev/null \
        | grep -q "IP Address:${PUBLIC_IP}" || return 1
    [[ "$(openssl x509 -in "$TLS_DIR/panel.crt" -noout -pubkey | openssl sha256)" \
        == "$(openssl pkey -in "$TLS_DIR/panel.key" -pubout | openssl sha256)" ]]
}

create_tls_cert() {
    if cert_is_valid; then
        info "existing certificate is valid; keeping it"
    else
        info "creating a secure certificate for $PUBLIC_IP"
        rm -f "$TLS_DIR/panel.crt" "$TLS_DIR/panel.key"
        openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes \
            -days 3650 -subj "/CN=zenex-panel" \
            -addext "subjectAltName=IP:${PUBLIC_IP},IP:127.0.0.1" \
            -keyout "$TLS_DIR/panel.key" -out "$TLS_DIR/panel.crt" >/dev/null 2>&1
    fi
    fix_file_permissions
}

install_unit() {
    cat > "$UNIT_PATH" <<EOF
[Unit]
Description=Zenex Panel API
After=network-online.target postgresql.service zenex-helper.service
Wants=network-online.target postgresql.service zenex-helper.service
StartLimitIntervalSec=0

[Service]
Type=simple
User=${ZENEX_USER}
Group=${ZENEX_USER}
EnvironmentFile=${ENV_FILE}
ExecStart=${BIN_PATH} serve
WorkingDirectory=${STATE_DIR}
Restart=always
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
ReadWritePaths=${STATE_DIR} ${LOG_DIR} -${RUN_DIR}
ReadOnlyPaths=${TLS_DIR}
CapabilityBoundingSet=
AmbientCapabilities=
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
    systemctl daemon-reload
}

configure_firewall() {
    local ssh_port
    ssh_port="$(sshd -T 2>/dev/null | awk '/^port /{print $2; exit}' || true)"
    ssh_port="${ssh_port:-22}"

    ufw --force default deny incoming >/dev/null
    ufw --force default allow outgoing >/dev/null
    ufw allow 22/tcp comment 'ssh' >/dev/null
    ufw allow "${ssh_port}/tcp" comment 'ssh' >/dev/null
    ufw allow 80/tcp comment 'http' >/dev/null
    ufw allow 443/tcp comment 'https' >/dev/null
    ufw allow "${PANEL_PORT}/tcp" comment 'zenex panel' >/dev/null
    ufw --force enable >/dev/null
    info "firewall on (SSH ${ssh_port}, web 80/443, panel ${PANEL_PORT})"
}

start_base_services() {
    local svc
    PHP_VER="$(php -r 'echo PHP_MAJOR_VERSION, ".", PHP_MINOR_VERSION;')"
    for svc in postgresql nginx mariadb redis-server fail2ban "php${PHP_VER}-fpm"; do
        if ! retry 3 5 systemctl enable --now "$svc"; then
            fail "could not start $svc. Run: systemctl status $svc"
        fi
    done
    info "web, database and cache services running"
}

# ---------------------------------------------------------------------------
# Start the panel, with automatic repair
# ---------------------------------------------------------------------------

panel_healthy() {
    local body
    body="$(curl -ksf --max-time 4 "https://127.0.0.1:${PANEL_PORT}/api/v1/health" 2>/dev/null || true)"
    [[ "$body" == *'"database":"ok"'* ]]
}

wait_for_panel() {
    local i
    for i in $(seq 1 20); do
        if panel_healthy; then
            return 0
        fi
        sleep 2
    done
    return 1
}

repair_panel_from_logs() {
    local logs
    # Only this attempt's messages. Older lines from previous runs would
    # otherwise send the repair down the wrong path.
    logs="$(journalctl -u zenex-api --since "$ATTEMPT_START" --no-pager 2>/dev/null || true)"

    if grep -qi "permission denied" <<<"$logs"; then
        info "fixing file access for the panel service"
        fix_file_permissions
    elif grep -qiE "panel\.(crt|key)" <<<"$logs"; then
        info "the certificate is missing or unreadable; creating a new one"
        create_tls_cert
    elif grep -qiE "password authentication failed|connection refused|does not exist|no pg_hba|connect: " <<<"$logs"; then
        info "repairing database access"
        setup_database
        write_env_file
    elif grep -qi "address already in use" <<<"$logs"; then
        info "the port is taken by another program; choosing a free port"
        PANEL_PORT=""
        choose_panel_port
        write_env_file
        configure_firewall
    elif grep -qiE "no such file or directory|exec format error" <<<"$logs"; then
        info "rebuilding the panel program"
        build_api
    else
        info "no known fix matched; restarting"
    fi
}

# Confirms the service user can read every file it needs, including every
# folder on the way there. Fixes the folders first, then checks again.
verify_service_access() {
    local f
    for f in "$TLS_DIR/panel.crt" "$TLS_DIR/panel.key" "$BIN_PATH"; do
        if ! runuser -u "$ZENEX_USER" -- test -r "$f"; then
            info "the panel service cannot read $f yet; fixing folder access"
            fix_file_permissions
            if ! runuser -u "$ZENEX_USER" -- test -r "$f"; then
                fail "the panel service still cannot read $f. Send the output of: ls -ld $CONF_DIR $TLS_DIR"
            fi
        fi
    done
    info "panel service can read its files"
}

ATTEMPT_START=""

start_panel() {
    local attempt
    verify_service_access
    for attempt in 1 2 3; do
        ATTEMPT_START="$(date '+%Y-%m-%d %H:%M:%S')"
        systemctl enable zenex-api >/dev/null 2>&1 || true
        systemctl restart zenex-api || true
        if wait_for_panel; then
            info "panel is running"
            return 0
        fi
        info "the panel is not ready yet (try $attempt of 3). Looking for the cause..."
        repair_panel_from_logs
        sleep 3
    done
    printf '\n      Last messages from the panel:\n'
    journalctl -u zenex-api --since "$ATTEMPT_START" --no-pager -n 15 2>/dev/null | sed 's/^/      /' || true
    fail "the panel did not start after automatic repairs. The messages above show the cause."
}

# ---------------------------------------------------------------------------
# Administrator account and final checks
# ---------------------------------------------------------------------------

ensure_admin() {
    if [[ -f "$CREDS_FILE" ]]; then
        info "administrator already set up. Login details are in $CREDS_FILE"
        return 0
    fi
    ADMIN_PASSWORD_NEW="$(openssl rand -hex 12)"
    printf '%s\n%s\n' "$ZENEX_ADMIN_EMAIL" "$ADMIN_PASSWORD_NEW" | (
        set -a
        # shellcheck disable=SC1090
        . "$ENV_FILE"
        set +a
        "$BIN_PATH" create-admin
    ) >/dev/null

    (
        umask 077
        cat > "$CREDS_FILE" <<EOF
Zenex panel administrator
Address:  https://${PUBLIC_IP}:${PANEL_PORT}
Email:    ${ZENEX_ADMIN_EMAIL}
Password: ${ADMIN_PASSWORD_NEW}

Change this password after your first sign-in.
This file is readable only by root.
EOF
    )
    info "administrator account created"
}

check_item() {
    local label=$1
    shift
    if "$@" >/dev/null 2>&1; then
        printf '      [OK]   %s\n' "$label"
    else
        printf '      [FAIL] %s\n' "$label"
        return 1
    fi
}

final_checks() {
    local failed=0
    check_item "panel program is running" systemctl is-active --quiet zenex-api || failed=1
    check_item "panel answers and database is connected" panel_healthy || failed=1
    check_item "database server (PostgreSQL) is running" systemctl is-active --quiet postgresql || failed=1
    check_item "web server (nginx) is running" systemctl is-active --quiet nginx || failed=1
    check_item "web server configuration is valid" nginx -t || failed=1
    check_item "database for sites (MariaDB) is running" systemctl is-active --quiet mariadb || failed=1
    check_item "cache (Redis) is running" systemctl is-active --quiet redis-server || failed=1
    check_item "firewall is on" bash -c 'ufw status | grep -q "Status: active"' || failed=1
    check_item "website helper is running" systemctl is-active --quiet zenex-helper || failed=1
    check_item "WP-CLI is installed" bash -c "php '$WP_BIN' --info >/dev/null 2>&1" || failed=1

    if (( failed )); then
        fail "some checks did not pass (see [FAIL] above). Re-run the same command; it repairs most issues automatically."
    fi
}

print_summary() {
    local login_line
    if [[ -n "$ADMIN_PASSWORD_NEW" ]]; then
        login_line="Password: ${ADMIN_PASSWORD_NEW}"
    else
        login_line="Password: (unchanged; shown in $CREDS_FILE)"
    fi

    cat <<EOF

============================================================
 DONE. The Zenex panel is installed and running.

 Open this address in your browser:

     https://${PUBLIC_IP}:${PANEL_PORT}

 Your browser will show a security warning because the
 certificate is created on this server. This is expected.
 Click "Advanced" and then "Continue" to open the panel.

 Sign in with:
     Email:    ${ZENEX_ADMIN_EMAIL}
     ${login_line}

 Login details are also saved in: ${CREDS_FILE}
 Install log: ${LOG_FILE}

 Re-running this command is safe. It keeps your data.

 Available now: sign-in, live server dashboard, connect domains, create WordPress websites.
 Coming next: SSL, DNS automation, separate servers.
============================================================
EOF
}

# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

main() {
    require_root
    mkdir -p "$LOG_DIR"
    exec > >(tee -a "$LOG_FILE") 2>&1
    printf '\n===== Zenex install started %s =====\n' "$(date '+%Y-%m-%d %H:%M:%S %Z')"

    step "Checking this server"
    check_os
    check_disk
    check_internet

    step "Preparing memory (swap if needed)"
    ensure_swap

    step "Installing system software (this takes a few minutes)"
    install_packages

    step "Preparing the service account and folders"
    create_service_user
    prepare_directories

    step "Downloading the Zenex panel"
    fetch_source

    step "Building the Zenex panel"
    build_api || fail "the panel could not be built. Check your internet connection and run the command again."

    step "Installing website tools (WP-CLI and the website helper)"
    install_wp_cli
    build_helper || fail "the website helper could not be built. Check your internet connection and run the command again."
    install_helper_unit
    start_helper

    step "Finding your server IP and a free port"
    detect_public_ip
    choose_panel_port

    step "Setting up the panel database"
    setup_database
    write_env_file

    step "Creating the secure certificate"
    create_tls_cert

    step "Configuring the firewall"
    configure_firewall

    step "Starting web, database and cache services"
    start_base_services

    step "Starting the Zenex panel"
    install_unit
    start_panel

    step "Creating your administrator account"
    ensure_admin

    step "Running final checks"
    final_checks

    print_summary
}

main "$@"
