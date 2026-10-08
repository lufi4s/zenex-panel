"use strict";

// Every API call sends the anti-CSRF header the server requires for non-GET requests.
const BASE_HEADERS = { "X-Requested-With": "zenex" };
const METRICS_MS = 5000;
const JOB_MS = 2000;
const STATE_TEXT = {
  ready: "Live", provisioning: "Building", failed: "Failed", suspended: "Suspended",
  deleting: "Deleting", deleted: "Deleted",
};

const $ = (id) => document.getElementById(id);
let metricsTimer = null;
let jobTimer = null;
let activeJobId = null;
let activeSiteId = null;
let openSiteId = null;
let openSite = null;

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

function button(label, onClick, className = "ghost") {
  const b = el("button", label, className);
  b.type = "button";
  b.addEventListener("click", onClick);
  return b;
}

// ---------------------------------------------------------------------------
// Session
// ---------------------------------------------------------------------------

function showLogin() {
  stopTimers();
  $("app-view").hidden = true;
  $("login-view").hidden = false;
  $("login-form").password.value = "";
}

function showApp(user) {
  $("login-view").hidden = true;
  $("app-view").hidden = false;
  $("who").textContent = user.email;
  refreshMetrics();
  metricsTimer = setInterval(refreshMetrics, METRICS_MS);
  loadDomains();
  loadSites();
}

function stopTimers() {
  if (metricsTimer) { clearInterval(metricsTimer); metricsTimer = null; }
  if (jobTimer) { clearInterval(jobTimer); jobTimer = null; }
}

// ---------------------------------------------------------------------------
// Server strip
// ---------------------------------------------------------------------------

function fmtBytes(n) {
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let v = n, i = 0;
  while (v >= 1024 && i < units.length - 1) { v /= 1024; i++; }
  return `${v.toFixed(i === 0 ? 0 : 1)} ${units[i]}`;
}

function fmtUptime(sec) {
  const d = Math.floor(sec / 86400), h = Math.floor((sec % 86400) / 3600), m = Math.floor((sec % 3600) / 60);
  return d > 0 ? `${d}d ${h}h` : `${h}h ${m}m`;
}

async function refreshMetrics() {
  const res = await api("/api/v1/system/metrics");
  if (res.status === 401) { showLogin(); return; }
  if (!res.ok) { $("metrics-error").textContent = errorText(res, "Server status unavailable."); return; }
  const m = res.body;
  $("metrics-error").textContent = "";
  $("m-load").textContent = `Load ${m.load_1m.toFixed(2)}`;
  $("m-mem").textContent = `Memory ${fmtBytes(m.mem_used_bytes)} / ${fmtBytes(m.mem_total_bytes)}`;
  $("m-disk").textContent = `Disk ${fmtBytes(m.disk_used_bytes)} / ${fmtBytes(m.disk_total_bytes)}`;
  $("m-uptime").textContent = `Up ${fmtUptime(m.uptime_seconds)}`;
}

// ---------------------------------------------------------------------------
// Domain
// ---------------------------------------------------------------------------

async function loadDomains() {
  const res = await api("/api/v1/domains");
  if (res.status === 401) { showLogin(); return; }
  const domains = res.ok ? res.body : [];
  const list = $("domain-list");
  list.replaceChildren();
  if (domains.length === 0) list.appendChild(el("li", "No domain added yet.", "muted"));

  domains.forEach((d) => {
    const li = el("li", undefined, "row");
    const head = el("div", undefined, "row-head");
    head.append(el("strong", d.apex), el("span", d.verified ? "DNS OK" : "DNS not set", d.verified ? "badge badge-ok" : "badge badge-bad"));
    head.appendChild(button("Check DNS", async (e) => {
      e.currentTarget.disabled = true;
      await api(`/api/v1/domains/${d.id}/verify`, { method: "POST" });
      loadDomains();
    }));
    li.appendChild(head);
    if (d.message) li.appendChild(el("p", d.message, d.verified ? "note" : "error"));
    list.appendChild(li);
  });

  const select = $("site-apex");
  select.replaceChildren();
  domains.forEach((d) => {
    const opt = el("option", d.verified ? d.apex : `${d.apex} (DNS not set)`);
    opt.value = d.apex;
    select.appendChild(opt);
  });
  select.disabled = domains.length === 0;
  $("site-form").querySelector("button").disabled = domains.length === 0;
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
    $("domain-error").textContent = errorText(res, "Could not add the domain.");
  }
});

// ---------------------------------------------------------------------------
// New website
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
  const submit = form.querySelector("button");
  submit.disabled = true;
  const res = await api("/api/v1/sites", {
    method: "POST",
    headers: { "Idempotency-Key": crypto.randomUUID() },
    body: JSON.stringify({ label: form.label.value.trim().toLowerCase(), apex: form.apex.value }),
  });
  submit.disabled = false;
  if (res.ok) {
    form.label.value = "";
    updatePreview();
    watchJob(res.body.job_id, res.body.site.id);
    loadSites();
  } else {
    $("site-error").textContent = errorText(res, "Could not create the website.");
  }
});

function renderSteps(steps) {
  const list = $("progress-steps");
  list.replaceChildren();
  steps.forEach((st) => {
    const li = el("li", undefined, `step step-${st.status}`);
    li.append(el("span", st.name.replaceAll("_", " ")), el("span", st.status, "step-status"));
    list.appendChild(li);
  });
}

async function pollJob() {
  if (!activeJobId) return;
  const res = await api(`/api/v1/jobs/${activeJobId}`);
  if (!res.ok) return;
  const { job, steps } = res.body;
  renderSteps(steps);
  if (job.status === "succeeded") {
    $("progress-title").textContent = "Website is live";
    $("progress-error").textContent = "";
    $("retry-job").hidden = true;
    stopJobPolling();
    loadSites();
  } else if (job.status === "failed") {
    $("progress-title").textContent = "Build stopped";
    $("progress-error").textContent = job.error || "The build stopped.";
    $("retry-job").hidden = false;
    stopJobPolling();
    loadSites();
  }
}

function stopJobPolling() {
  if (jobTimer) { clearInterval(jobTimer); jobTimer = null; }
}

function watchJob(jobId, siteId) {
  stopJobPolling();
  activeJobId = jobId;
  activeSiteId = siteId;
  $("progress").hidden = false;
  $("progress-title").textContent = "Building website";
  $("progress-error").textContent = "";
  $("retry-job").hidden = true;
  pollJob();
  jobTimer = setInterval(pollJob, JOB_MS);
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
// Websites
// ---------------------------------------------------------------------------

async function loadSites() {
  const res = await api("/api/v1/sites");
  if (res.status === 401) { showLogin(); return; }
  const sites = res.ok ? res.body : [];
  const list = $("site-list");
  list.replaceChildren();
  $("sites-empty").hidden = sites.length > 0;

  sites.forEach((s) => {
    const li = el("li", undefined, "row");
    const head = el("div", undefined, "row-head");
    if (s.state === "ready") {
      const a = el("a", `http://${s.domain}`);
      a.href = `http://${s.domain}`;
      a.target = "_blank";
      a.rel = "noopener";
      head.appendChild(a);
    } else {
      head.appendChild(el("strong", s.domain));
    }
    head.appendChild(el("span", STATE_TEXT[s.state] || s.state, `badge state-${s.state}`));
    head.appendChild(button(openSiteId === s.id ? "Close" : "Manage", () => {
      openSiteId = openSiteId === s.id ? null : s.id;
      loadSites();
    }));
    li.appendChild(head);

    if (openSiteId === s.id) {
      openSite = s;
      li.appendChild(managePanel(s));
    }
    list.appendChild(li);
  });
}

// Everything a customer can do with one website, shown inline under its row.
function managePanel(site) {
  const box = el("div", undefined, "manage");
  const ready = site.state === "ready";
  const suspended = site.state === "suspended";
  const msg = el("p", "", "error");
  const logBox = el("pre", undefined, "log");
  logBox.hidden = true;

  const actions = el("div", undefined, "actions");
  const run = async (path, method = "POST", body) => {
    msg.textContent = "";
    const res = await api(path, body === undefined ? { method } : { method, body: JSON.stringify(body) });
    if (!res.ok) { msg.textContent = errorText(res, "That did not work."); return null; }
    return res.body;
  };

  if (ready) {
    actions.appendChild(button("WordPress login", async () => {
      const res = await api(`/api/v1/sites/${site.id}/credentials`);
      if (!res.ok) { msg.textContent = errorText(res, "Could not load the login."); return; }
      cred.replaceChildren(
        el("div", `Dashboard: ${res.body.url}`),
        el("div", `Username: ${res.body.username}`),
        el("div", `Password: ${res.body.password}`),
      );
      cred.hidden = false;
    }));
    actions.appendChild(button("Suspend", async () => {
      if (await run(`/api/v1/sites/${site.id}/suspend`)) loadSites();
    }));
    actions.appendChild(button("Restart PHP", async () => {
      if (await run(`/api/v1/sites/${site.id}/php-restart`)) msg.textContent = "PHP restarted.";
    }));
  }
  if (suspended) {
    actions.appendChild(button("Resume", async () => {
      if (await run(`/api/v1/sites/${site.id}/resume`)) loadSites();
    }));
  }
  actions.appendChild(button("Log", async () => {
    const res = await api(`/api/v1/sites/${site.id}/logs`);
    if (!res.ok) { msg.textContent = errorText(res, "Could not read the log."); return; }
    logBox.textContent = res.body.log || "Nothing logged yet.";
    logBox.hidden = false;
  }));
  box.appendChild(actions);

  const cred = el("div", undefined, "cred");
  cred.hidden = true;
  box.appendChild(cred);

  if (ready) {
    const phpForm = el("form", undefined, "inline");
    const select = el("select");
    select.setAttribute("aria-label", "PHP version");
    const current = site.php_version;
    api("/api/v1/php-versions").then((res) => {
      const versions = res.ok ? res.body.versions : [current];
      versions.forEach((v) => {
        const opt = el("option", `PHP ${v}`);
        opt.value = v;
        if (v === current) opt.selected = true;
        select.appendChild(opt);
      });
    });
    const change = el("button", "Change PHP", "ghost");
    change.type = "submit";
    phpForm.append(select, change);
    phpForm.addEventListener("submit", async (e) => {
      e.preventDefault();
      if (select.value === current) { msg.textContent = "Already on this PHP version."; return; }
      if (await run(`/api/v1/sites/${site.id}/php`, "POST", { version: select.value })) loadSites();
    });
    box.appendChild(phpForm);
  }

  box.appendChild(msg);
  box.appendChild(logBox);

  const danger = el("form", undefined, "danger inline wrap");
  const confirm = el("input");
  confirm.placeholder = `type ${site.domain} to delete`;
  confirm.setAttribute("aria-label", "Confirm deletion");
  const del = el("button", "Delete website", "danger-btn");
  del.type = "submit";
  danger.append(confirm, del);
  danger.addEventListener("submit", async (e) => {
    e.preventDefault();
    if (confirm.value.trim().toLowerCase() !== site.domain.toLowerCase()) {
      msg.textContent = `Type ${site.domain} exactly to confirm.`;
      return;
    }
    const res = await api(`/api/v1/sites/${site.id}`, { method: "DELETE" });
    if (!res.ok) { msg.textContent = errorText(res, "Could not delete."); return; }
    openSiteId = null;
    watchJob(res.body.job_id, site.id);
    loadSites();
  });
  box.appendChild(danger);
  return box;
}

// ---------------------------------------------------------------------------
// Session controls
// ---------------------------------------------------------------------------

$("login-form").addEventListener("submit", async (e) => {
  e.preventDefault();
  const form = e.currentTarget;
  const submit = form.querySelector("button");
  $("login-error").textContent = "";
  submit.disabled = true;
  const res = await api("/api/v1/auth/login", {
    method: "POST",
    body: JSON.stringify({ email: form.email.value.trim(), password: form.password.value }),
  });
  submit.disabled = false;
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
