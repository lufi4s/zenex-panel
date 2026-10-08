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
    expect(await screen.findByText("shop.ozima.cloud")).toBeTruthy();
    expect(screen.getAllByText("ozima.cloud").length).toBeGreaterThan(0);
    expect(screen.getByText("0.42")).toBeTruthy();
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
