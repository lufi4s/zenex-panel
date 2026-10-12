import { useEffect, type ReactNode } from "react";
import { LogOut } from "lucide-react";
import { ApiError } from "@/lib/api";
import { useLogout, useMe } from "@/lib/queries";
import { NAV } from "@/components/app/nav";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";
import { Button } from "@/components/ui/button";
import { Separator } from "@/components/ui/separator";
import {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
  SidebarTrigger,
} from "@/components/ui/sidebar";
import { Skeleton } from "@/components/ui/skeleton";
import { Toaster } from "@/components/ui/sonner";

/**
 * The frame every signed-in page shares: the sidebar, the top bar and the page. A visitor who is
 * not signed in is sent to the sign-in page.
 */
export function AppShell({ active, title, children }: { active: string; title: string; children: ReactNode }) {
  const me = useMe();
  const logout = useLogout();

  const signedOut = me.error instanceof ApiError && me.error.status === 401;
  useEffect(() => {
    if (signedOut) window.location.replace("/login");
  }, [signedOut]);

  const signOut = () => {
    logout.mutate(undefined, { onSettled: () => window.location.replace("/login") });
  };

  if (me.isPending || signedOut) {
    return (
      <div className="flex min-h-svh items-center justify-center">
        <Skeleton className="h-8 w-48" />
      </div>
    );
  }

  const initial = me.data?.email?.[0]?.toUpperCase() ?? "?";

  return (
    <SidebarProvider>
      <Sidebar variant="inset" collapsible="icon">
        <SidebarHeader>
          <div className="flex items-center gap-2 px-2 py-1.5">
            <div className="flex size-8 items-center justify-center rounded-lg bg-primary text-sm font-semibold text-primary-foreground">
              Z
            </div>
            <div className="grid text-sm leading-tight group-data-[collapsible=icon]:hidden">
              <span className="font-semibold">Zenex</span>
              <span className="text-xs text-muted-foreground">Panel</span>
            </div>
          </div>
        </SidebarHeader>
        <SidebarContent>
          <SidebarGroup>
            <SidebarGroupContent>
              <SidebarMenu>
                {NAV.map((item) => (
                  <SidebarMenuItem key={item.href}>
                    <SidebarMenuButton
                      render={<a href={item.href} />}
                      isActive={item.title === active}
                      tooltip={item.title}
                    >
                      <item.icon />
                      <span>{item.title}</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                ))}
              </SidebarMenu>
            </SidebarGroupContent>
          </SidebarGroup>
        </SidebarContent>
        <SidebarFooter>
          <div className="flex items-center gap-2 px-2 py-1.5">
            <Avatar className="size-7">
              <AvatarFallback>{initial}</AvatarFallback>
            </Avatar>
            <span className="min-w-0 flex-1 truncate text-xs text-muted-foreground group-data-[collapsible=icon]:hidden">
              {me.data?.email}
            </span>
            <Button variant="ghost" size="icon-sm" aria-label="Sign out" onClick={signOut}>
              <LogOut />
            </Button>
          </div>
        </SidebarFooter>
      </Sidebar>

      <SidebarInset>
        <header className="sticky top-0 z-10 flex h-14 items-center gap-2 border-b bg-background/70 px-4 backdrop-blur-md">
          <SidebarTrigger className="-ml-1" />
          <Separator orientation="vertical" className="mr-2 h-4" />
          <h1 className="text-sm font-medium">{title}</h1>
        </header>
        <main className="mx-auto w-full max-w-7xl flex-1 p-4 sm:p-6">{children}</main>
      </SidebarInset>
      <Toaster />
    </SidebarProvider>
  );
}
