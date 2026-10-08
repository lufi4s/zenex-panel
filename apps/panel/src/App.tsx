import { useEffect, type ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes, useLocation } from "react-router";
import { useBranding, useMe } from "@/api/queries";
import { ApiError, describeError } from "@/api/client";
import { applyBranding } from "@/lib/brand";
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
    <main className="flex min-h-dvh items-center justify-center text-sm text-muted-foreground">
      Loading…
    </main>
  );
}

function ConnectionError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <main className="flex min-h-dvh flex-col items-center justify-center gap-3 p-6 text-center">
      <p className="text-sm text-destructive">{message}</p>
      <button
        type="button"
        className="rounded-md border border-border px-3 py-1.5 text-sm hover:bg-muted"
        onClick={onRetry}
      >
        Try again
      </button>
    </main>
  );
}

/** Sign-in for visitors; the panel for everyone else. */
function LoginRoute() {
  const me = useMe();
  if (me.isPending) return <Loading />;
  if (me.data) return <Navigate to="/" replace />;
  if (me.error instanceof ApiError && me.error.status !== 401) {
    return <ConnectionError message={describeError(me.error)} onRetry={() => me.refetch()} />;
  }
  return <LoginPage />;
}

/** Protects every page: visitors without a session are sent to sign-in. */
function ProtectedRoute() {
  const me = useMe();
  const location = useLocation();
  if (me.isPending) return <Loading />;
  if (me.error instanceof ApiError && me.error.status === 401) {
    return <Navigate to="/login" replace state={{ from: location.pathname }} />;
  }
  if (!me.data)
    return <ConnectionError message={describeError(me.error)} onRetry={() => me.refetch()} />;
  return <AppLayout />;
}

/** Settings are for administrators only. */
function AdminRoute({ children }: { children: ReactNode }) {
  const me = useMe();
  if (me.data && !me.data.roles.includes("administrator")) return <Navigate to="/" replace />;
  return <>{children}</>;
}

function NotFound() {
  return (
    <div className="flex flex-col items-start gap-2 py-16">
      <p className="font-heading text-2xl font-semibold">Page not found</p>
      <p className="text-sm text-muted-foreground">The page you are looking for does not exist.</p>
    </div>
  );
}

/** The application: routes, the saved branding, and the signed-in frame. */
export function App() {
  const branding = useBranding();

  useEffect(() => {
    if (branding.data) applyBranding(branding.data);
  }, [branding.data]);

  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<LoginRoute />} />
        <Route element={<ProtectedRoute />}>
          <Route index element={<OverviewPage />} />
          <Route path="websites" element={<WebsitesPage />} />
          <Route path="websites/:id" element={<SitePage />} />
          <Route path="domains" element={<DomainsPage />} />
          <Route path="server" element={<ServerPage />} />
          <Route path="activity" element={<ActivityPage />} />
          <Route
            path="settings"
            element={
              <AdminRoute>
                <SettingsPage />
              </AdminRoute>
            }
          />
          <Route path="*" element={<NotFound />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
