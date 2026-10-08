import { useEffect, useState, type ReactNode } from "react";
import {
  Activity,
  Globe,
  Layers,
  LayoutDashboard,
  LineChart,
  LogOut,
  ScrollText,
  Server,
} from "lucide-react";
import { useLogout } from "@/api/queries";
import type { User } from "@/api/types";
import { NotificationBell } from "@/components/NotificationBell";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

export const SECTIONS = [
  { id: "overview", label: "Overview", icon: LayoutDashboard },
  { id: "monitoring", label: "Server history", icon: LineChart },
  { id: "services", label: "Services", icon: Server },
  { id: "domains", label: "Domains", icon: Globe },
  { id: "websites", label: "Websites", icon: Layers },
  { id: "activity", label: "Activity", icon: ScrollText },
] as const;

type SectionId = (typeof SECTIONS)[number]["id"];

/** Which section is in view, so the menu can highlight it while scrolling. */
function useActiveSection(): SectionId {
  const [active, setActive] = useState<SectionId>("overview");
  useEffect(() => {
    // Older or test browsers may lack IntersectionObserver; the menu then simply keeps its default.
    if (typeof IntersectionObserver === "undefined") return;
    const observer = new IntersectionObserver(
      (entries) => {
        const visible = entries
          .filter((e) => e.isIntersecting)
          .sort((a, b) => a.boundingClientRect.top - b.boundingClientRect.top);
        const first = visible[0];
        if (first) setActive(first.target.id as SectionId);
      },
      { rootMargin: "-20% 0px -65% 0px" },
    );
    SECTIONS.forEach((s) => {
      const node = document.getElementById(s.id);
      if (node) observer.observe(node);
    });
    return () => observer.disconnect();
  }, []);
  return active;
}

function NavLink({
  id,
  label,
  icon: Icon,
  active,
  compact,
}: {
  id: SectionId;
  label: string;
  icon: typeof Activity;
  active: boolean;
  compact?: boolean;
}) {
  return (
    <a
      href={`#${id}`}
      aria-current={active ? "location" : undefined}
      className={cn(
        "flex items-center gap-2.5 rounded-md text-sm font-medium transition-colors",
        compact ? "shrink-0 px-3 py-1.5" : "px-3 py-2",
        active
          ? "bg-primary/10 text-primary"
          : "text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      <Icon className="size-4 shrink-0" aria-hidden />
      {label}
    </a>
  );
}

/**
 * The page frame: a sidebar on wide screens, a horizontal menu on small ones,
 * and a top bar with notifications. The content scrolls; the frame stays put.
 */
export function AppShell({ user, children }: { user: User; children: ReactNode }) {
  const logout = useLogout();
  const active = useActiveSection();

  return (
    <div className="min-h-dvh bg-background">
      <aside className="fixed inset-y-0 left-0 hidden w-60 flex-col border-r border-border bg-card lg:flex">
        <div className="flex h-14 items-center gap-2.5 border-b border-border px-5">
          <span className="flex size-7 items-center justify-center rounded-md bg-primary text-sm font-bold text-primary-foreground">
            Z
          </span>
          <span className="font-semibold tracking-tight">Zenex</span>
        </div>
        <nav aria-label="Sections" className="flex flex-1 flex-col gap-0.5 p-3">
          {SECTIONS.map((s) => (
            <NavLink key={s.id} id={s.id} label={s.label} icon={s.icon} active={active === s.id} />
          ))}
        </nav>
        <div className="border-t border-border p-3">
          <div className="mb-2 truncate px-2 text-xs text-muted-foreground" title={user.email}>
            {user.email}
          </div>
          <Button
            variant="ghost"
            size="sm"
            className="w-full justify-start"
            disabled={logout.isPending}
            onClick={() => logout.mutate()}
          >
            <LogOut aria-hidden />
            Sign out
          </Button>
        </div>
      </aside>

      <div className="lg:pl-60">
        <header className="sticky top-0 z-20 flex h-14 items-center gap-3 border-b border-border bg-card/90 px-4 backdrop-blur sm:px-6">
          <span
            className="flex size-7 items-center justify-center rounded-md bg-primary text-sm font-bold text-primary-foreground lg:hidden"
            aria-hidden
          >
            Z
          </span>
          <span className="font-semibold tracking-tight lg:hidden">Zenex</span>
          <span className="hidden text-sm text-muted-foreground lg:inline">Control panel</span>
          <span className="ml-auto" />
          <NotificationBell />
          <Button
            variant="ghost"
            size="icon"
            className="lg:hidden"
            aria-label="Sign out"
            disabled={logout.isPending}
            onClick={() => logout.mutate()}
          >
            <LogOut aria-hidden />
          </Button>
        </header>

        <nav
          aria-label="Sections"
          className="flex gap-1 overflow-x-auto border-b border-border bg-card px-3 py-2 lg:hidden"
        >
          {SECTIONS.map((s) => (
            <NavLink
              key={s.id}
              id={s.id}
              label={s.label}
              icon={s.icon}
              active={active === s.id}
              compact
            />
          ))}
        </nav>

        <main className="mx-auto flex w-full max-w-6xl flex-col gap-8 px-4 py-6 sm:px-6 lg:py-8">
          {children}
        </main>
      </div>
    </div>
  );
}
