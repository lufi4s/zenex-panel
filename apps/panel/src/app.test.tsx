// @vitest-environment jsdom
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, render, screen, waitFor } from "@testing-library/react";
import { QueryClientProvider } from "@tanstack/react-query";
import { App } from "./App";
import { queryClient } from "./api/queries";

// Answers the API calls the dashboard makes, using the same JSON shapes as the server.
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
    if (url.endsWith("/api/v1/system/metrics")) return json(metrics);
    if (url.endsWith("/api/v1/domains")) return json(domains);
    if (url.endsWith("/api/v1/sites")) return json(sites);
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
    if (url.endsWith("/api/v1/php-versions")) return json({ versions: ["8.3"] });
    return json({ error: { code: "not_found", message: "not found" } }, 404);
  });
}

function renderApp() {
  return render(
    <QueryClientProvider client={queryClient}>
      <App />
    </QueryClientProvider>,
  );
}

describe("panel", () => {
  beforeEach(() => {
    queryClient.clear();
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows sign-in when there is no session", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: false }));
    renderApp();
    expect(await screen.findByText("Sign in to Zenex")).toBeTruthy();
  });

  it("signs in and shows the single-page dashboard", async () => {
    const fetchMock = mockApi({ signedIn: false });
    vi.stubGlobal("fetch", fetchMock);
    renderApp();

    fireEvent.change(await screen.findByLabelText("Email"), {
      target: { value: "admin@example.com" },
    });
    fireEvent.change(screen.getByLabelText("Password"), {
      target: { value: "correct horse battery" },
    });
    fireEvent.click(screen.getByRole("button", { name: "Sign in" }));

    expect(await screen.findByText("Your websites")).toBeTruthy();
    expect(screen.getByText("Your domain")).toBeTruthy();
    expect(screen.getByText("New website")).toBeTruthy();
    expect((await screen.findAllByText("shop.ozima.cloud")).length).toBeGreaterThan(0);
    expect(screen.getAllByText("ozima.cloud").length).toBeGreaterThan(0);
    expect(screen.getByText("0.42")).toBeTruthy();
    expect((await screen.findAllByText("Services")).length).toBeGreaterThan(0);
    expect((await screen.findAllByText("Caddy (web server)")).length).toBeGreaterThan(0);
    expect((await screen.findAllByText("Created website")).length).toBeGreaterThan(0);
    expect((await screen.findAllByText("Sign-in failed")).length).toBeGreaterThan(0);
  });

  it("sends the anti-CSRF header on sign-in", async () => {
    const fetchMock = mockApi({ signedIn: false });
    vi.stubGlobal("fetch", fetchMock);
    renderApp();

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

  it("opens a website's actions in place when Manage is pressed", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderApp();

    const manage = await screen.findByRole("button", { name: /Manage/ });
    fireEvent.click(manage);
    expect(await screen.findByRole("button", { name: "Restart PHP" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "Delete" })).toBeTruthy();
  });
});

describe("notifications and domain removal", () => {
  beforeEach(() => {
    queryClient.clear();
  });

  afterEach(() => {
    cleanup();
    vi.unstubAllGlobals();
  });

  it("shows the unread count on the bell", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderApp();
    expect(await screen.findByRole("button", { name: "Notifications, 2 unread" })).toBeTruthy();
  });

  it("opens the notification list", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderApp();
    fireEvent.click(await screen.findByRole("button", { name: "Notifications, 2 unread" }));
    expect(await screen.findByText("Website build failed")).toBeTruthy();
    expect(screen.getByText("Could not download WordPress.")).toBeTruthy();
  });

  it("keeps Remove domain disabled until the domain name is typed", async () => {
    vi.stubGlobal("fetch", mockApi({ signedIn: true }));
    renderApp();
    fireEvent.click(await screen.findByRole("button", { name: /Remove/ }));
    const confirm = await screen.findByRole("button", { name: "Remove domain" });
    expect((confirm as HTMLButtonElement).disabled).toBe(true);
    fireEvent.change(screen.getByLabelText(/Type/), { target: { value: "ozima.cloud" } });
    expect(
      (screen.getByRole("button", { name: "Remove domain" }) as HTMLButtonElement).disabled,
    ).toBe(false);
  });
});
