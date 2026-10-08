#!/usr/bin/env bash
# Updates the Zenex panel to the latest commit of main.
# Started by the root helper as the transient unit zenex-update, so it keeps
# running while the panel API and helper restart during the install.
# Progress is written to UPDATE_LOG; the last "== update" line tells the panel how it ended.
set -Eeuo pipefail

readonly REPO="/opt/zenex/src"
readonly UPDATE_LOG="/var/log/zenex/update.log"

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
