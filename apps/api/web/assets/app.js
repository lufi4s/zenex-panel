"use strict";

// Every API call sends the anti-CSRF header the server requires for non-GET requests.
const BASE_HEADERS = { "X-Requested-With": "zenex" };
const POLL_MS = 5000;
const JOB_POLL_MS = 2000;

const $ = (id) => document.getElementById(id);
let metricsTimer = null;
let jobTimer = null;
let activeJobId = null;
let activeSiteId = null;
let managedSite = null;

async function api(path, options = {}) {
  const headers = { ...BASE_HEADERS, ...(options.headers || {}) };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  const res = await fetch(path, { credentials: "same-origin", ...options, headers });
  let body = null;
  if (res.status !== 204) {
    try { body = await res.json(); } catch (_) { body = null; }
  }
  return { ok: res.ok, status: res.status, body };
}

function errorText(res, fallback) {
  return (res.body && res.body.error && res.body.error.message) || fallback;
}

function el(tag, text, className) {
  const node = document.createElement(tag);
  if (text !== undefined) node.textContent = text;
  if (className) node.className = className;
  return node;
}

// ---------------------------------------------------------------------------
// Views
// ---------------------------------------------------------------------------

function showLogin() {
  stopPolling();
  $("app-view").hidden = true;
  $("login-view").hidden = false;
  $("login-form").password.value = "";
}

function showApp(user) {
  $("login-view").hidden = true;
  $("app-view").hidden = false;
  $("who").textContent = user.email;
  selectTab("dashboard");
}

function selectTab(name) {
  document.querySelectorAll(".tab").forEach((b) => {
    b.setAttribute("aria-current", b.dataset.tab === name ? "page" : "false");
  });
  $("tab-dashboard").hidden = name !== "dashboard";
  $("tab-websites").hidden = name !== "websites";
  stopPolling();
  if (name === "dashboard") {
    refreshMetrics();
    metricsTimer = setInterval(refreshMetrics, POLL_MS);
  } else {
    loadDomains();
    loadSites();
  }
}

function stopPolling() {
  if (metricsTimer) { clearInterval(metricsTimer); metricsTimer = null; }
  if (jobTimer) { clearInterval(jobTimer); jobTimer = null; }
}

// ---------------------------------------------------------------------------
// Dashboard
// ---------------------------------------------------------------------------

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

// ---------------------------------------------------------------------------
// Domains and DNS verification
// ---------------------------------------------------------------------------

let domains = [];

function domainRow(d) {
  const li = el("li", undefined, "domain-item");
  const head = el("div", undefined, "domain-head");
  head.append(
    el("strong", d.apex),
    el("span", d.verified ? "DNS verified" : "Not pointing here", d.verified ? "badge badge-ok" : "badge badge-bad"),
  );
  const check = el("button", "Check again", "ghost");
  check.type = "button";
  check.addEventListener("click", async () => {
    check.disabled = true;
    await api(`/api/v1/domains/${d.id}/verify`, { method: "POST" });
    check.disabled = false;
    loadDomains();
  });
  head.appendChild(check);
  li.appendChild(head);
  if (d.message) li.appendChild(el("p", d.message, d.verified ? "note" : "error"));
  return li;
}

async function loadDomains() {
  const res = await api("/api/v1/domains");
  if (res.status === 401) { showLogin(); return; }
  domains = res.ok ? res.body : [];
  const list = $("domain-list");
  list.replaceChildren();
  domains.forEach((d) => list.appendChild(domainRow(d)));
  if (domains.length === 0) list.appendChild(el("li", "No domains connected yet.", "muted"));

  const select = $("site-apex");
  select.replaceChildren();
  domains.forEach((d) => {
    const opt = el("option", d.verified ? d.apex : `${d.apex} (not verified)`);
    opt.value = d.apex;
    select.appendChild(opt);
  });
  select.disabled = domains.length === 0;
  updatePreview();
}

$("domain-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = e.currentTarget;
  $("domain-error").textContent = "";
  const res = await api("/api/v1/domains", {
    method: "POST",
    body: JSON.stringify({ apex: form.apex.value.trim().toLowerCase() }),
  });
  if (res.ok) {
    form.apex.value = "";
    loadDomains();
  } else {
    $("domain-error").textContent = errorText(res, "Could not connect the domain.");
  }
});

// ---------------------------------------------------------------------------
// Create website
// ---------------------------------------------------------------------------

function updatePreview() {
  const label = $("site-form").label.value.trim().toLowerCase();
  const apex = $("site-apex").value;
  $("site-preview").textContent = label && apex ? `Address: http://${label}.${apex}` : "";
}

$("site-form").label.addEventListener("input", updatePreview);
$("site-apex").addEventListener("change", updatePreview);

$("site-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = e.currentTarget;
  $("site-error").textContent = "";
  const button = form.querySelector("button");
  button.disabled = true;
  const res = await api("/api/v1/sites", {
    method: "POST",
    headers: { "Idempotency-Key": crypto.randomUUID() },
    body: JSON.stringify({ label: form.label.value.trim().toLowerCase(), apex: form.apex.value }),
  });
  button.disabled = false;
  if (res.ok) {
    form.label.value = "";
    updatePreview();
    watchJob(res.body.job_id, res.body.site.id);
    loadSites();
  } else {
    $("site-error").textContent = errorText(res, "Could not create the website.");
  }
});

// ---------------------------------------------------------------------------
// Sites list
// ---------------------------------------------------------------------------

const STATE_TEXT = {
  ready: "Ready", provisioning: "Building", failed: "Failed", suspended: "Suspended",
  deleting: "Deleting", deleted: "Deleted",
};

async function loadSites() {
  const res = await api("/api/v1/sites");
  if (res.status === 401) { showLogin(); return; }
  const rows = $("site-rows");
  rows.replaceChildren();
  const sites = res.ok ? res.body : [];
  $("sites-empty").hidden = sites.length > 0;
  sites.forEach((s) => {
    const tr = el("tr");
    const addr = el("td");
    if (s.state === "ready") {
      const a = el("a", `http://${s.domain}`);
      a.href = `http://${s.domain}`;
      a.target = "_blank";
      a.rel = "noopener";
      addr.appendChild(a);
    } else {
      addr.textContent = s.domain;
    }
    const status = el("td", STATE_TEXT[s.state] || s.state, `state state-${s.state}`);
    const actions = el("td", undefined, "actions");
    const manage = el("button", "Manage", "ghost");
    manage.type = "button";
    manage.addEventListener("click", () => openManage(s.id));
    actions.appendChild(manage);
    tr.append(addr, status, actions);
    rows.appendChild(tr);
  });
}

// ---------------------------------------------------------------------------
// Build progress
// ---------------------------------------------------------------------------

function renderSteps(steps) {
  const list = $("progress-steps");
  list.replaceChildren();
  steps.forEach((st) => {
    const label = st.name.replaceAll("_", " ");
    const li = el("li", undefined, `step step-${st.status}`);
    li.append(el("span", label), el("span", st.status, "step-status"));
    list.appendChild(li);
  });
}

function showProgress(title) {
  $("progress-card").hidden = false;
  $("progress-title").textContent = title;
  $("progress-error").textContent = "";
  $("retry-job").hidden = true;
}

async function pollJob() {
  if (!activeJobId) return;
  const res = await api(`/api/v1/jobs/${activeJobId}`);
  if (!res.ok) return;
  const { job, steps } = res.body;
  renderSteps(steps);
  if (job.status === "succeeded") {
    $("progress-title").textContent = "Website is ready";
    finishWatch();
    loadSites();
  } else if (job.status === "failed") {
    $("progress-title").textContent = "Website build stopped";
    $("progress-error").textContent = job.error || "The build stopped. Retry to continue.";
    $("retry-job").hidden = false;
    finishWatch();
    loadSites();
  }
}

// Stops polling but keeps activeJobId, so "Retry" still knows which job to retry.
function finishWatch() {
  if (jobTimer) { clearInterval(jobTimer); jobTimer = null; }
}

function watchJob(jobId, siteId) {
  finishWatch();
  activeJobId = jobId;
  activeSiteId = siteId;
  showProgress("Building website");
  pollJob();
  jobTimer = setInterval(pollJob, JOB_POLL_MS);
}

$("retry-job").addEventListener("click", async () => {
  if (!activeJobId) return;
  const res = await api(`/api/v1/jobs/${activeJobId}/retry`, { method: "POST" });
  if (res.ok) {
    watchJob(activeJobId, activeSiteId);
  } else {
    $("progress-error").textContent = errorText(res, "Could not retry.");
  }
});

// ---------------------------------------------------------------------------
// Manage a website
// ---------------------------------------------------------------------------

function setManageError(msg) { $("manage-error").textContent = msg || ""; }

function renderManage(site) {
  managedSite = site;
  $("manage-card").hidden = false;
  $("manage-title").textContent = site.domain;
  const badge = $("manage-state");
  badge.textContent = STATE_TEXT[site.state] || site.state;
  badge.className = `badge ${site.state === "ready" ? "badge-ok" : site.state === "failed" ? "badge-bad" : ""}`;
  $("manage-info").textContent = `PHP ${site.php_version} · Account ${site.slug} · Created ${new Date(site.created_at).toLocaleString()}`;

  const ready = site.state === "ready";
  const suspended = site.state === "suspended";
  $("m-login").disabled = !ready;
  $("m-open").disabled = !ready;
  $("m-restart").disabled = !ready;
  $("m-logs").disabled = false;
  $("m-suspend").hidden = suspended;
  $("m-suspend").disabled = !ready;
  $("m-resume").hidden = !suspended;
  $("php-select").disabled = !ready;
  $("delete-form").confirm.placeholder = `type ${site.domain} to confirm`;
  setManageError("");
}

async function openManage(siteId) {
  const res = await api(`/api/v1/sites/${siteId}`);
  if (!res.ok) { $("site-error").textContent = errorText(res, "Could not open the website."); return; }
  $("manage-log").hidden = true;
  renderManage(res.body.site);
  await loadPHPVersions(res.body.site.php_version);
  $("manage-card").scrollIntoView({ behavior: "smooth", block: "start" });
}

async function loadPHPVersions(current) {
  const res = await api("/api/v1/php-versions");
  const select = $("php-select");
  select.replaceChildren();
  const versions = res.ok ? res.body.versions : [current];
  versions.forEach((v) => {
    const opt = el("option", `PHP ${v}`);
    opt.value = v;
    if (v === current) opt.selected = true;
    select.appendChild(opt);
  });
}

async function refreshManaged() {
  if (!managedSite) return;
  const res = await api(`/api/v1/sites/${managedSite.id}`);
  if (res.ok) renderManage(res.body.site);
  loadSites();
}

async function runSiteAction(path, body) {
  setManageError("");
  const res = await api(path, body === undefined ? { method: "POST" } : { method: "POST", body: JSON.stringify(body) });
  if (!res.ok) {
    setManageError(errorText(res, "The action did not complete."));
    return null;
  }
  return res.body;
}

$("manage-close").addEventListener("click", () => {
  $("manage-card").hidden = true;
  managedSite = null;
});

$("m-login").addEventListener("click", async () => {
  if (!managedSite) return;
  const res = await api(`/api/v1/sites/${managedSite.id}/credentials`);
  if (!res.ok) { setManageError(errorText(res, "Could not load the login.")); return; }
  $("cred-url").textContent = `Dashboard: ${res.body.url}`;
  $("cred-user").textContent = `Username: ${res.body.username}`;
  $("cred-pass").textContent = `Password: ${res.body.password}`;
  $("cred-box").hidden = false;
  $("cred-box").scrollIntoView({ behavior: "smooth", block: "nearest" });
});

$("cred-hide").addEventListener("click", () => {
  $("cred-box").hidden = true;
  $("cred-pass").textContent = "";
});

$("m-open").addEventListener("click", () => {
  if (managedSite) window.open(`http://${managedSite.domain}`, "_blank", "noopener");
});

$("m-suspend").addEventListener("click", async () => {
  if (managedSite && await runSiteAction(`/api/v1/sites/${managedSite.id}/suspend`)) refreshManaged();
});

$("m-resume").addEventListener("click", async () => {
  if (managedSite && await runSiteAction(`/api/v1/sites/${managedSite.id}/resume`)) refreshManaged();
});

$("m-restart").addEventListener("click", async () => {
  if (!managedSite) return;
  if (await runSiteAction(`/api/v1/sites/${managedSite.id}/php-restart`)) {
    setManageError("PHP restarted. Other websites on the same PHP version were restarted too.");
  }
});

$("m-logs").addEventListener("click", async () => {
  if (!managedSite) return;
  const res = await api(`/api/v1/sites/${managedSite.id}/logs`);
  const box = $("manage-log");
  if (!res.ok) { setManageError(errorText(res, "Could not read the log.")); return; }
  box.textContent = res.body.log || "No errors recorded yet.";
  box.hidden = false;
});

$("php-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  if (!managedSite) return;
  const version = $("php-select").value;
  if (version === managedSite.php_version) { setManageError("The website already uses this PHP version."); return; }
  const res = await runSiteAction(`/api/v1/sites/${managedSite.id}/php`, { version });
  if (res) { renderManage(res); }
});

$("delete-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  if (!managedSite) return;
  const typed = e.currentTarget.confirm.value.trim().toLowerCase();
  if (typed !== managedSite.domain.toLowerCase()) {
    setManageError(`Type ${managedSite.domain} exactly to confirm.`);
    return;
  }
  const res = await api(`/api/v1/sites/${managedSite.id}`, { method: "DELETE" });
  if (!res.ok) { setManageError(errorText(res, "Could not delete the website.")); return; }
  $("manage-card").hidden = true;
  managedSite = null;
  e.currentTarget.confirm.value = "";
  loadSites();
});

// ---------------------------------------------------------------------------
// Navigation and session
// ---------------------------------------------------------------------------

document.querySelectorAll(".tab").forEach((b) => {
  b.addEventListener("click", () => selectTab(b.dataset.tab));
});

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
    showApp(res.body);
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
  if (res.ok) { showApp(res.body); } else { showLogin(); }
})();
