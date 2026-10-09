// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { App } from "./App";
import { resetQueryCache } from "./api/query";

// Answers the API calls the panel makes, using the same JSON shapes as the server.
function mockApi(options: { signedIn: boolean }) {
  const json = (body: unknown, status = 200) =>
    Promise.resolve(
      new Response(JSON.stringify(body), {
        status,
        headers: { "Content-Type": "application/json" },
      }),
    );

  const user = { id: "u1", email: "admin@example.com", roles: ["administrator"] };
  const domains = [
    {
      id: "d1",
      apex: "ozima.cloud",
      verified: true,
      message: "DNS verified",
      created_at: "2026-01-01T00:00:00Z",
    },
  ];
  const sites = [
    {
      id: "s1",
      owner_user_id: "u1",
      node_id: "n1",
      slug: "shop",
      domain: "shop.ozima.cloud",
      php_version: "8.3",
      state: "ready",
      health: "healthy",
      created_at: "2026-01-02T00:00:00Z",
    },
  ];
  const metrics = {
    cpu_count: 2,
    load_1m: 0.42,
    load_5m: 0.3,
    load_15m: 0.2,
    mem_total_bytes: 4 * 1024 ** 3,
    mem_used_bytes: 1024 ** 3,
    disk_total_bytes: 20 * 1024 ** 3,
    disk_used_bytes: 5 * 1024 ** 3,
    uptime_seconds: 90061,
  };

  return vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    const url = String(input);
    const method = init?.method ?? "GET";
    if (url.endsWith("/api/v1/auth/me")) {
      return options.signedIn
        ? json(user)
        : json({ error: { code: "unauthorized", message: "sign in required" } }, 401);
    }
    if (url.endsWith("/api/v1/auth/login") && method === "POST") return json(user);
    if (url.endsWith("/api/v1/branding")) {
      return json({ name: "Zenex Panel", tagline: "", primary_color: "#0f766e" });
    }
    if (url.endsWith("/api/v1/system/metrics")) return json(metrics);
    if (url.endsWith("/api/v1/domains")) return json(domains);
    if (url.endsWith("/api/v1/sites") && method === "POST") {
      return json({
        site: {
          ...sites[0],
          id: "s2",
          slug: "blog",
          domain: "blog.ozima.cloud",
          state: "provisioning",
        },
        job_id: "j1",
      });
    }
    if (url.endsWith("/api/v1/sites")) return json(sites);
    if (url.endsWith("/api/v1/jobs/j1/logs")) {
      return json([
        { id: 1, time: "2026-10-08T10:00:00Z", level: "info", message: "Creating site account" },
      ]);
    }
    if (url.endsWith("/api/v1/jobs/j1")) {
      return json({
        job: { id: "j1", type: "provision", status: "running", attempts: 1 },
        steps: [
          { name: "create_account", status: "running", attempts: 1 },
          { name: "download_wordpress", status: "pending", attempts: 0 },
        ],
      });
    }
    if (url.endsWith("/api/v1/sites/s1")) return json({ site: sites[0] });
    if (url.endsWith("/api/v1/sites/s1/logs")) {
      return json({ log: "203.0.113.5 - GET / 200\n203.0.113.9 - GET /wp-login.php 200" });
    }
    if (url.includes("/api/v1/monitoring/metrics")) {
      return json({
        points: [
          { t: "2026-10-08T10:00:00Z", load: 0.3, cpus: 2, mem_pct: 25, disk_pct: 20 },
          { t: "2026-10-08T10:01:00Z", load: 0.5, cpus: 2, mem_pct: 26, disk_pct: 20 },
        ],
        bucket_seconds: 60,
      });
    }
    if (url.includes("/api/v1/monitoring/services")) {
      return json([
        { name: "caddy", state: "active" },
        { name: "postgresql", state: "inactive" },
      ]);
    }
    if (url.includes("/api/v1/monitoring/sites")) return json([]);
    if (url.includes("/api/v1/activity")) {
      return json({
        items: [
          {
            id: 9,
            time: "2026-10-08T10:00:00Z",
            action: "site.create",
            target_type: "site",
            target_id: "s1",
            result: "success",
          },
          {
            id: 8,
            time: "2026-10-08T09:00:00Z",
            action: "login.failure",
            result: "failure",
            error_code: "invalid_credentials",
          },
        ],
      });
    }
    if (url.includes("/api/v1/notifications")) {
      return json({
        unread: 2,
        items: [
          {
            id: 1,
            level: "error",
            title: "Website build failed",
            body: "Could not download WordPress.",
            created_at: "2026-10-08T10:00:00Z",
          },
          {
            id: 2,
            level: "success",
            title: "Website is live",
            body: "shop.ozima.cloud is ready.",
            created_at: "2026-10-08T09:00:00Z",
          },
        ],
      });
    }
    if (url.includes("/files?path=")) {
      return json([
        {
          name: "wp-content",
          type: "dir",
          size: 0,
          modified: "2026-10-08T10:00:00Z",
          editable: false,
        },
        {
          name: "index.php",
          type: "file",
          size: 120,
          modified: "2026-10-08T10:00:00Z",
          editable: true,
        },
      ]);
    }
    if (url.endsWith("/api/v1/system/update")) {
      return json({
        current: "f781dd8",
        latest: "532a16c",
        update_available: true,
        state: "idle",
        log: "",
      });
    }
    if (url.endsWith("/api/v1/php-versions")) return json({ versions: ["8.3"] });
    return json({ error: { code: "not_found", message: "not found" } }, 404);
  });
}

/** Opens the panel at a path, the way the browser would. */
function renderAt(path: string) {
  window.history.pushState({}, "", path);
  return render(<App />);
}

/** Tabs switch on mouse-down in the panel, so the test presses the same way a person does. */
function chooseTab(name: string) {
  const tab = screen.getByRole("tab", { name });
  fireEvent.mouseDown(tab, { button: 0 });
  fireEvent.click(tab);
}

beforeEach(() => {
  resetQueryCache();
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  window.history.pushState({}, "", "/");
});

describe("sign-in", () => {
  it("shows sign-in when there is no session", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: false }));
    renderAt("/");
    expect(await screen.findByText("Welcome back")).toBeTruthy();
  });

  it("signs in and opens the overview", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: false }));
    renderAt("/login");

    fireEvent.change(await screen.findByLabelText("Email"), {
      target: { value: "admin@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "correct horse battery" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByRole("heading", { name: "Overview" })).toBeTruthy();
    expect((await screen.findAllByText("shop.ozima.cloud")).length).toBeGreaterThan(0);
    expect(screen.getByText("0.42")).toBeTruthy();
    expect((await screen.findAllByText("Caddy (web server)")).length).toBeGreaterThan(0);
  });

  it("sends the anti-CSRF header on sign-in", async () => {
    const fetchMock = mockApi({ signedIn: false });
    vi.stubGlobal("fetch", fetchMock);
    renderAt("/login");

    fireEvent.change(await screen.findByLabelText("Email"), {
      target: { value: "admin@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "correct horse battery" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));

    await waitFor(() => {
      const loginCall = fetchMock.mock.calls.find(([url]) =>
        String(url).endsWith("/api/v1/auth/login"),
      );
      if (!loginCall) throw new Error("login request was not sent");
      const headers = (loginCall[1] as RequestInit).headers as Record<string, string>;
      expect(headers["X-Requested-With"]).toBe("zenex");
    });
  });
});

describe("navigation", () => {
  it("lists websites on their own page", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderAt("/websites");
    expect(await screen.findByRole("heading", { name: "Websites" })).toBeTruthy();
    expect(await screen.findByRole("link", { name: "Open shop.ozima.cloud" })).toBeTruthy();
    expect(screen.getByRole("button", { name: /New website/ })).toBeTruthy();
  });

  it("opens a website from the list and shows its page", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderAt("/websites");
    fireEvent.click(await screen.findByRole("link", { name: "Open shop.ozima.cloud" }));
    expect(await screen.findByRole("heading", { name: "shop.ozima.cloud" })).toBeTruthy();
    expect(await screen.findByRole("button", { name: "Restart PHP" })).toBeTruthy();
  });

  it("shows the files of a website on its Files tab", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderAt("/websites/s1");
    await screen.findByRole("heading", { name: "shop.ozima.cloud" });
    chooseTab("Files");
    expect(await screen.findByText("wp-content")).toBeTruthy();
    expect(await screen.findByText("index.php")).toBeTruthy();
  });

  it("shows the site log on its Logs tab", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderAt("/websites/s1");
    await screen.findByRole("heading", { name: "shop.ozima.cloud" });
    chooseTab("Logs");
    expect(await screen.findByText(/wp-login\.php/)).toBeTruthy();
  });

  it("shows the build steps in the popup after creating a website, without the log", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderAt("/websites");
    fireEvent.click(await screen.findByRole("button", { name: /New website/ }));
    fireEvent.change(await screen.findByLabelText("Subdomain name"), {
      target: { value: "blog" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Create website" }));

    expect(await screen.findByRole("heading", { name: "Building website" })).toBeTruthy();
    expect(await screen.findByText("0 of 2 steps complete")).toBeTruthy();
    expect(screen.queryByText("Creating site account")).toBeNull();
    expect(screen.queryByRole("button", { name: "Show log" })).toBeNull();
    expect(screen.getByRole("button", { name: "Open website" })).toBeTruthy();
  });

  it("keeps Remove domain disabled until the domain name is typed", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderAt("/domains");
    fireEvent.click(await screen.findByRole("button", { name: /Remove/ }));
    const confirm = await screen.findByRole("button", { name: "Remove domain" });
    expect((confirm as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText(/Type/), { target: { value: "ozima.cloud" } });
    expect(
      (screen.getByRole("button", { name: "Remove domain" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });

  it("offers the panel update on the settings page when a new version exists", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderAt("/settings");
    expect(await screen.findByText("Panel updates")).toBeTruthy();
    expect(await screen.findByRole("button", { name: /Update now/ })).toBeTruthy();
  });

  it("shows a not-found page for unknown addresses", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderAt("/does-not-exist");
    expect(await screen.findByText("Page not found")).toBeTruthy();
  });
});

describe("notifications", () => {
  it("shows the unread count on the bell", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderAt("/");
    expect(await screen.findByRole("button", { name: "Notifications, 2 unread" })).toBeTruthy();
  });

  it("opens the notification list", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderAt("/");
    fireEvent.click(await screen.findByRole("button", { name: "Notifications, 2 unread" }));
    expect(await screen.findByText("Website build failed")).toBeTruthy();
    expect(screen.getByText("Could not download WordPress.")).toBeTruthy();
  });
});
