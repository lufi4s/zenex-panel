import { useState, type ReactNode } from "react";
import { useMe } from "@/api/queries";
import { AppSidebar } from "@/components/app-sidebar";
import { Menu } from "@/components/icons";
import { NotificationBell } from "@/components/NotificationBell";

/** The signed-in frame: a sidebar on wide screens, a drawer on small ones, and the page. */
export function AppLayout({ children }: { children: ReactNode }) {
  const me = useMe();
  const [drawerOpen, setDrawerOpen] = useState(false);
  if (!me.data) return null;

  return (
    <div className="min-h-dvh bg-background lg:flex">
      {drawerOpen && (
        <div
          aria-hidden="true"
          className="fixed inset-0 z-30 bg-black/40 lg:hidden"
          onClick={() => setDrawerOpen(false)}
        />
      )}
      <AppSidebar user={me.data} open={drawerOpen} onNavigate={() => setDrawerOpen(false)} />

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="sticky top-0 z-20 flex h-14 shrink-0 items-center gap-2 border-b border-border bg-background/95 px-4 backdrop-blur">
          <button
            type="button"
            aria-label="Open menu"
            onClick={() => setDrawerOpen(true)}
            className="-ml-2 rounded-md p-2 text-muted-foreground hover:bg-muted hover:text-foreground lg:hidden"
          >
            <Menu />
          </button>
          <div className="ml-auto flex items-center gap-1">
            <NotificationBell />
          </div>
        </header>
        <main className="mx-auto flex w-full max-w-7xl flex-1 flex-col gap-6 p-4 sm:p-6 lg:p-8">
          {children}
        </main>
      </div>
    </div>
  );
}
