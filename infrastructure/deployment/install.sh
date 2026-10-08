#!/usr/bin/env bash
# Zenex VPS bootstrap.
#
# Prepares a fresh Ubuntu 24.04 LTS server for the Zenex stack:
#   - verifies OS and root access
#   - installs Nginx, PHP-FPM, MariaDB, Redis, UFW, Fail2Ban, Certbot
#   - sets a default-deny firewall that allows only SSH, HTTP and HTTPS
#   - creates Zenex state directories with restrictive permissions
#
# The Zenex Agent is NOT installed by this script yet; no signed release exists.
#
# Usage (as root):
#   curl -fsSL https://raw.githubusercontent.com/<owner>/zenex-panel/main/infrastructure/deployment/install.sh | sudo bash
#
# Safe to run more than once.

set -euo pipefail

readonly ZENEX_STATE_DIR="/var/lib/zenex"
readonly ZENEX_CONFIG_DIR="/etc/zenex"
readonly ZENEX_LOG_DIR="/var/log/zenex"

log()  { printf '[zenex] %s\n' "$*"; }
fail() { printf '[zenex] ERROR: %s\n' "$*" >&2; exit 1; }

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
    log "installing packages"
    export DEBIAN_FRONTEND=noninteractive
    apt-get update -qq
    apt-get install -y -qq --no-install-recommends \
        ca-certificates curl \
        nginx \
        php-fpm php-mysql php-curl php-gd php-mbstring php-xml php-zip php-intl php-redis \
        mariadb-server \
        redis-server \
        ufw \
        fail2ban \
        certbot python3-certbot-nginx
}

configure_firewall() {
    log "configuring firewall (default deny; allow 22, 80, 443)"
    ufw --force default deny incoming
    ufw --force default allow outgoing
    ufw allow 22/tcp comment 'ssh'
    ufw allow 80/tcp comment 'http'
    ufw allow 443/tcp comment 'https'
    ufw --force enable
}

enable_services() {
    log "enabling services"
    systemctl enable --now nginx
    systemctl enable --now mariadb
    systemctl enable --now redis-server
    systemctl enable --now fail2ban
    systemctl enable --now "php$(php -r 'echo PHP_MAJOR_VERSION.".".PHP_MINOR_VERSION;')-fpm"
}

create_state_dirs() {
    log "creating Zenex directories"
    install -d -m 0750 -o root -g root "$ZENEX_CONFIG_DIR"
    install -d -m 0750 -o root -g root "$ZENEX_STATE_DIR"
    install -d -m 0750 -o root -g adm  "$ZENEX_LOG_DIR"
}

verify_nginx() {
    log "validating nginx configuration"
    nginx -t
}

main() {
    require_root
    require_ubuntu_2404
    install_packages
    configure_firewall
    enable_services
    create_state_dirs
    verify_nginx
    log "bootstrap complete. Zenex Agent install is pending a signed release."
}

main "$@"
