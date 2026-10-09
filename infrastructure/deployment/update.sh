#!/usr/bin/env bash
# Updates the Zenex panel to the latest commit of main.
# Started by the root helper as the transient unit zenex-update, so it keeps
# running while the panel API and helper restart during the install.
# Progress is written to UPDATE_LOG; the last "== update" line tells the panel how it ended.
set -Eeuo pipefail

readonly REPO="/opt/zenex/src"
readonly UPDATE_LOG="/var/log/zenex/update.log"

# The transient unit starts with a bare environment. The installer's Go build needs
# HOME (for the module cache), and the usual Go and system paths.
export HOME=/root
export GOPATH=/root/go
export GOMODCACHE=/root/go/pkg/mod
export GOCACHE=/root/.cache/go-build
export PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin:/usr/local/go/bin

mkdir -p "$(dirname "$UPDATE_LOG")"
exec >"$UPDATE_LOG" 2>&1

echo "== update started $(date '+%Y-%m-%d %H:%M:%S')"

on_error() {
    echo "== update FAILED $(date '+%Y-%m-%d %H:%M:%S') (see the lines above)"
    exit 1
}
trap on_error ERR

echo "[update] fetching the latest version from GitHub"
git -C "$REPO" fetch -q --depth 1 origin main
git -C "$REPO" reset -q --hard FETCH_HEAD

echo "[update] installing the new version"
bash "$REPO/infrastructure/deployment/install.sh"

echo "== update finished OK $(date '+%Y-%m-%d %H:%M:%S')"
