import { useEffect } from "react";
import { useBranding, useMe } from "@/api/queries";
import { ApiError, describeError } from "@/api/client";
import { applyBranding } from "@/lib/brand";
import { LoginPage } from "@/pages/LoginPage";
import { Dashboard } from "@/pages/Dashboard";

/** Decides between sign-in and the panel, based on the session cookie. */
export function App() {
  const me = useMe();
  const branding = useBranding();

  // Apply the saved name and accent colour to the whole panel, including sign-in.
  useEffect(() => {
    if (branding.data) applyBranding(branding.data);
  }, [branding.data]);

  if (me.isPending) {
    return (
      <main className="flex min-h-dvh items-center justify-center text-sm text-muted-foreground">
        Loading…
      </main>
    );
  }

  if (me.data) return <Dashboard user={me.data} />;

  // An expired or missing session is the normal case: show sign-in.
  if (me.error instanceof ApiError && me.error.status === 401) return <LoginPage />;

  return (
    <main className="flex min-h-dvh flex-col items-center justify-center gap-3 p-6 text-center">
      <p className="text-sm text-destructive">{describeError(me.error)}</p>
      <button
        type="button"
        className="rounded-md border border-border px-3 py-1.5 text-sm hover:bg-muted"
        onClick={() => me.refetch()}
      >
        Try again
      </button>
    </main>
  );
}
