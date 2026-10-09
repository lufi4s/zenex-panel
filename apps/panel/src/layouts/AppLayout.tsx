import { createSignal, Show, type JSX } from "solid-js";
import { useMe } from "@/api/queries";
import { AppSidebar } from "@/components/app-sidebar";
import { Menu } from "@/components/icons";
import { NotificationBell } from "@/components/NotificationBell";

/** The signed-in frame: a sidebar on wide screens, a drawer on small ones, and the page. */
export function AppLayout(props: { children: JSX.Element }) {
  const me = useMe();
  const [drawerOpen, setDrawerOpen] = createSignal(false);

  return (
    <Show when={me.data}>
      {(user) => (
        <div class="min-h-dvh bg-background lg:flex">
          <Show when={drawerOpen()}>
            <div
              aria-hidden="true"
              class="fixed inset-0 z-30 bg-black/40 lg:hidden"
              onClick={() => setDrawerOpen(false)}
            />
          </Show>
          <AppSidebar user={user()} open={drawerOpen()} onNavigate={() => setDrawerOpen(false)} />

          <div class="flex min-w-0 flex-1 flex-col">
            <header class="sticky top-0 z-20 flex h-14 shrink-0 items-center gap-2 border-b border-border bg-background/95 px-4 backdrop-blur">
              <button
                type="button"
                aria-label="Open menu"
                onClick={() => setDrawerOpen(true)}
                class="-ml-2 rounded-md p-2 text-muted-foreground hover:bg-muted hover:text-foreground lg:hidden"
              >
                <Menu />
              </button>
              <div class="ml-auto flex items-center gap-1">
                <NotificationBell />
              </div>
            </header>
            <main class="mx-auto flex w-full max-w-7xl flex-1 flex-col gap-6 p-4 sm:p-6 lg:p-8">
              {props.children}
            </main>
          </div>
        </div>
      )}
    </Show>
  );
}
