import { createEffect, Show, type JSX } from "solid-js";
import { useBranding, useMe } from "@/api/queries";
import { ApiError, describeError } from "@/api/client";
import { applyBranding } from "@/lib/brand";
import { Navigate, Routes, useLocation, type RouteDef } from "@/lib/router";
import { AppLayout } from "@/layouts/AppLayout";
import { ActivityPage } from "@/pages/ActivityPage";
import { DomainsPage } from "@/pages/DomainsPage";
import { LoginPage } from "@/pages/LoginPage";
import { OverviewPage } from "@/pages/OverviewPage";
import { ServerPage } from "@/pages/ServerPage";
import { SettingsPage } from "@/pages/SettingsPage";
import { SitePage } from "@/pages/SitePage";
import { WebsitesPage } from "@/pages/WebsitesPage";

function Loading() {
  return (
    <main class="flex min-h-dvh items-center justify-center text-sm text-muted-foreground">
      Loading…
    </main>
  );
}

function ConnectionError(props: { message: string; onRetry: () => void }) {
  return (
    <main class="flex min-h-dvh flex-col items-center justify-center gap-3 p-6 text-center">
      <p class="text-sm text-destructive">{props.message}</p>
      <button
        type="button"
        class="rounded-md border border-border px-3 py-1.5 text-sm hover:bg-muted"
        onClick={props.onRetry}
      >
        Try again
      </button>
    </main>
  );
}

/** Sign-in for visitors; the panel for everyone else. */
function LoginRoute() {
  const me = useMe();
  return (
    <Show when={!me.isPending} fallback={<Loading />}>
      <Show when={!me.data} fallback={<Navigate to="/" replace />}>
        <Show
          when={!(me.error instanceof ApiError && me.error.status !== 401)}
          fallback={
            <ConnectionError message={describeError(me.error)} onRetry={() => me.refetch()} />
          }
        >
          <LoginPage />
        </Show>
      </Show>
    </Show>
  );
}

/** Protects every page: visitors without a session are sent to sign-in. */
function Protected(props: { children: JSX.Element }) {
  const me = useMe();
  return (
    <Show when={!me.isPending} fallback={<Loading />}>
      <Show
        when={!(me.error instanceof ApiError && me.error.status === 401)}
        fallback={<Navigate to="/login" replace />}
      >
        <Show
          when={me.data}
          fallback={
            <ConnectionError message={describeError(me.error)} onRetry={() => me.refetch()} />
          }
        >
          <AppLayout>{props.children}</AppLayout>
        </Show>
      </Show>
    </Show>
  );
}

/** Settings are for administrators only. */
function AdminOnly(props: { children: JSX.Element }) {
  const me = useMe();
  return (
    <Show
      when={!me.data || me.data.roles.includes("administrator")}
      fallback={<Navigate to="/" replace />}
    >
      {props.children}
    </Show>
  );
}

function NotFound() {
  return (
    <div class="flex flex-col items-start gap-2 py-16">
      <p class="font-heading text-2xl font-semibold">Page not found</p>
      <p class="text-sm text-muted-foreground">The page you are looking for does not exist.</p>
    </div>
  );
}

const ROUTES: RouteDef[] = [
  { path: "/login", component: () => <LoginRoute /> },
  {
    path: "/",
    component: () => (
      <Protected>
        <OverviewPage />
      </Protected>
    ),
  },
  {
    path: "/websites",
    component: () => (
      <Protected>
        <WebsitesPage />
      </Protected>
    ),
  },
  {
    path: "/websites/:id",
    component: () => (
      <Protected>
        <SitePage />
      </Protected>
    ),
  },
  {
    path: "/domains",
    component: () => (
      <Protected>
        <DomainsPage />
      </Protected>
    ),
  },
  {
    path: "/server",
    component: () => (
      <Protected>
        <ServerPage />
      </Protected>
    ),
  },
  {
    path: "/activity",
    component: () => (
      <Protected>
        <ActivityPage />
      </Protected>
    ),
  },
  {
    path: "/settings",
    component: () => (
      <Protected>
        <AdminOnly>
          <SettingsPage />
        </AdminOnly>
      </Protected>
    ),
  },
];

/** The application: routes, the saved branding, and the signed-in frame. */
export function App() {
  const branding = useBranding();
  const location = useLocation();

  createEffect(() => {
    if (branding.data) applyBranding(branding.data);
  });

  return (
    // Routes keeps the params of the route it first matched, so it is rebuilt for each address.
    <Show when={location.pathname} keyed>
      {(_pathname) => (
        <Routes
          routes={ROUTES}
          fallback={() => (
            <Protected>
              <NotFound />
            </Protected>
          )}
        />
      )}
    </Show>
  );
}
