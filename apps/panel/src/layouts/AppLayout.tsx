import { Outlet } from "react-router";
import { useMe } from "@/api/queries";
import { AppSidebar } from "@/components/app-sidebar";
import { NotificationBell } from "@/components/NotificationBell";
import { Separator } from "@/components/ui/separator";
import { SidebarInset, SidebarProvider, SidebarTrigger } from "@/components/ui/sidebar";

/** The signed-in frame: sidebar on the left, a slim header, and the page content. */
export function AppLayout() {
  const me = useMe();
  if (!me.data) return null;

  return (
    <SidebarProvider>
      <AppSidebar user={me.data} />
      <SidebarInset>
        <header className="sticky top-0 z-10 flex h-14 shrink-0 items-center gap-2 border-b border-border bg-background/95 px-4 backdrop-blur supports-[backdrop-filter]:bg-background/80">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-1 h-4" />
          <div className="ml-auto flex items-center gap-1">
            <NotificationBell />
          </div>
        </header>
        <main className="mx-auto flex w-full max-w-7xl flex-1 flex-col gap-6 p-4 sm:p-6 lg:p-8">
          <Outlet />
        </main>
      </SidebarInset>
    </SidebarProvider>
  );
}
