"use strict";

// Every API call sends the anti-CSRF header the server requires for non-GET requests.
const HEADERS = { "X-Requested-With": "zenex", "Content-Type": "application/json" };
const POLL_MS = 5000;

const $ = (id) => document.getElementById(id);
let pollTimer = null;

async function api(path, options = {}) {
  const res = await fetch(path, { credentials: "same-origin", headers: HEADERS, ...options });
  let body = null;
  if (res.status !== 204) {
    try { body = await res.json(); } catch (_) { body = null; }
  }
  return { ok: res.ok, status: res.status, body };
}

function errorText(res, fallback) {
  return (res.body && res.body.error && res.body.error.message) || fallback;
}

function showLogin() {
  stopPolling();
  $("dashboard-view").hidden = true;
  $("login-view").hidden = false;
  $("login-form").password.value = "";
}

function showDashboard(user) {
  $("login-view").hidden = true;
  $("dashboard-view").hidden = false;
  $("who").textContent = user.email;
  startPolling();
}

function fmtBytes(n) {
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let v = n, i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function fmtUptime(sec) {
  const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
  return d > 0 ? `${d}d ${h}h ${m}m` : `${h}h ${m}m`;
}

function pct(part, whole) {
  return whole > 0 ? `${((part / whole) * 100).toFixed(0)}% used` : "";
}

async function refreshMetrics() {
  const res = await api("/api/v1/system/metrics");
  if (res.status === 401) { showLogin(); return; }
  if (!res.ok) { $("metrics-error").textContent = errorText(res, "Could not read server metrics."); return; }
  const m = res.body;
  $("metrics-error").textContent = "";
  $("m-load").textContent = `${m.load_1m.toFixed(2)} / ${m.load_5m.toFixed(2)} / ${m.load_15m.toFixed(2)}`;
  $("m-cpus").textContent = `${m.cpu_count} CPU${m.cpu_count === 1 ? "" : "s"}`;
  $("m-mem").textContent = `${fmtBytes(m.mem_used_bytes)} / ${fmtBytes(m.mem_total_bytes)}`;
  $("m-mem-pct").textContent = pct(m.mem_used_bytes, m.mem_total_bytes);
  $("m-disk").textContent = `${fmtBytes(m.disk_used_bytes)} / ${fmtBytes(m.disk_total_bytes)}`;
  $("m-disk-pct").textContent = pct(m.disk_used_bytes, m.disk_total_bytes);
  $("m-uptime").textContent = fmtUptime(m.uptime_seconds);
}

function startPolling() {
  stopPolling();
  refreshMetrics();
  pollTimer = setInterval(refreshMetrics, POLL_MS);
}

function stopPolling() {
  if (pollTimer) { clearInterval(pollTimer); pollTimer = null; }
}

$("login-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = e.currentTarget;
  const button = form.querySelector("button");
  $("login-error").textContent = "";
  button.disabled = true;
  const res = await api("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email: form.email.value.trim(), password: form.password.value }),
  });
  button.disabled = false;
  if (res.ok) {
    form.password.value = "";
    showDashboard(res.body);
  } else {
    $("login-error").textContent = errorText(res, "Sign-in failed.");
  }
});

$("logout").addEventListener("click", async () => {
  await api("/api/v1/auth/logout", { method: "POST" });
  showLogin();
});

(async function init() {
  const res = await api("/api/v1/auth/me");
  if (res.ok) { showDashboard(res.body); } else { showLogin(); }
})();
