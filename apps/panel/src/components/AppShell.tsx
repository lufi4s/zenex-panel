import { useEffect, useState, type ReactNode } from "react";
import {
  Activity,
  Globe,
  Layers,
  LayoutDashboard,
  LineChart,
  LogOut,
  Palette,
  ScrollText,
  Server,
} from "lucide-react";
import { useBranding, useLogout } from "@/api/queries";
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

/** Administrators also get a Branding section. */
const ADMIN_SECTION = { id: "branding", label: "Branding", icon: Palette } as const;

type SectionId = (typeof SECTIONS)[number]["id"] | "branding";

/** Which section is in view, so the menu can highlight it while scrolling. */
function useActiveSection(): SectionId {
  const [active, setActive] = useState<SectionId>("overview");
  useEffect(() => {
    // Older or test browsers may lack IntersectionObserver; the menu then keeps its default.
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
}: {
  id: SectionId;
  label: string;
  icon: typeof Activity;
  active: boolean;
}) {
  return (
    <a
      href={`#${id}`}
      aria-current={active ? "location" : undefined}
      className={cn(
        "flex h-full shrink-0 items-center gap-2 whitespace-nowrap border-b-2 px-1 text-sm font-medium transition-colors",
        active
          ? "bg-primary/10 text-primary"
          : "text-muted-foreground hover:bg-muted hover:text-foreground",
      )}
    >
      <Icon className="size-4" aria-hidden />
      {label}
    </a>
  );
}

/**
 * The page frame: one top bar with the brand, the section links, notifications
 * and sign-out. The content uses the full width and scrolls beneath it.
 */
export function AppShell({ user, children }: { user: User; children: ReactNode }) {
  const logout = useLogout();
  const branding = useBranding();
  const sections = user.roles.includes("administrator") ? [...SECTIONS, ADMIN_SECTION] : SECTIONS;
  const name = branding.data?.name ?? "Zenex Panel";
  const active = useActiveSection();

  return (
    <div className="min-h-dvh bg-background">
      <header className="sticky top-0 z-20 border-b border-border bg-card">
        <div className="mx-auto flex h-14 max-w-7xl items-center gap-4 px-4 sm:px-6">
          <div className="flex shrink-0 items-center gap-2.5">
            <span
              className="flex size-7 items-center justify-center rounded-md bg-primary text-sm font-bold text-primary-foreground"
              aria-hidden
            >
              {name.trim()[0]?.toUpperCase() ?? "Z"}
            </span>
            <span className="font-semibold tracking-tight">{name}</span>
          </div>

          <nav
            aria-label="Sections"
            className="hidden min-w-0 flex-1 items-center gap-1 overflow-x-auto md:flex"
          >
            {sections.map((s) => (
              <NavLink
                key={s.id}
                id={s.id}
                label={s.label}
                icon={s.icon}
                active={active === s.id}
              />
            ))}
          </nav>

          <div className="ml-auto flex items-center gap-1 md:gap-2">
            <span
              className="hidden max-w-[200px] truncate text-sm text-muted-foreground xl:inline"
              title={user.email}
            >
              {user.email}
            </span>
            <NotificationBell />
            <Button
              variant="ghost"
              size="sm"
              disabled={logout.isPending}
              onClick={() => logout.mutate()}
              aria-label="Sign out"
            >
              <LogOut aria-hidden />
              <span className="hidden sm:inline">Sign out</span>
            </Button>
          </div>
        </div>

        <nav
          aria-label="Sections"
          className="flex gap-1 overflow-x-auto border-t border-border px-3 py-2 md:hidden"
        >
          {sections.map((s) => (
            <NavLink key={s.id} id={s.id} label={s.label} icon={s.icon} active={active === s.id} />
          ))}
        </nav>
      </header>

      <main className="mx-auto flex w-full max-w-7xl flex-col gap-8 px-4 py-5 sm:gap-10 sm:px-6 sm:py-8">
        {children}
      </main>
    </div>
  );
}
