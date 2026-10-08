import {
  Activity,
  Globe,
  Layers,
  LayoutDashboard,
  LogOut,
  Palette,
  Server,
} from "@/components/icons";
import { useBranding, useLogout } from "@/api/queries";
import type { User } from "@/api/types";
import { NavLink, useNavigate } from "@/lib/router";
import { cn } from "@/lib/utils";

const PLATFORM = [
  { to: "/", label: "Overview", icon: LayoutDashboard, end: true },
  { to: "/websites", label: "Websites", icon: Layers, end: false },
  { to: "/domains", label: "Domains", icon: Globe, end: false },
];

const OPERATIONS = [
  { to: "/server", label: "Server", icon: Server, end: false },
  { to: "/activity", label: "Activity", icon: Activity, end: false },
];

const LINK =
  "flex items-center gap-3 rounded-md px-3 py-2 text-sm transition-colors hover:bg-muted hover:text-foreground [&_svg]:size-4 [&_svg]:shrink-0";

function Group({
  label,
  items,
  onNavigate,
}: {
  label: string;
  items: { to: string; label: string; icon: typeof Server; end: boolean }[];
  onNavigate: () => void;
}) {
  return (
    <div className="space-y-1">
      <p className="px-3 pb-1 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {label}
      </p>
      {items.map((item) => (
        <NavLink
          key={item.to}
          to={item.to}
          end={item.end}
          onClick={onNavigate}
          className={cn(
            LINK,
            "data-[active]:bg-muted data-[active]:font-medium data-[active]:text-foreground",
            "text-muted-foreground",
          )}
        >
          <item.icon />
          <span>{item.label}</span>
        </NavLink>
      ))}
    </div>
  );
}

/** The left navigation. On small screens it slides in as a drawer. */
export function AppSidebar({
  user,
  open,
  onNavigate,
}: {
  user: User;
  open: boolean;
  onNavigate: () => void;
}) {
  const branding = useBranding();
  const logout = useLogout();
  const navigate = useNavigate();
  const name = branding.data?.name ?? "Zenex Panel";
  const isAdmin = user.roles.includes("administrator");
  const initial = user.email.charAt(0).toUpperCase();

  const signOut = () => {
    logout.mutate(undefined, { onSettled: () => navigate("/login", { replace: true }) });
  };

  return (
    <aside
      aria-label="Main navigation"
      className={cn(
        "fixed inset-y-0 left-0 z-40 flex w-64 flex-col border-r border-border bg-card transition-transform duration-200",
        "lg:sticky lg:top-0 lg:h-dvh lg:shrink-0 lg:translate-x-0",
        open ? "translate-x-0" : "-translate-x-full",
      )}
    >
      <div className="flex h-14 items-center gap-2.5 border-b border-border px-4">
        <span
          aria-hidden="true"
          className="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground"
        >
          {name.trim().charAt(0).toUpperCase() || "Z"}
        </span>
        <span className="truncate font-heading text-base font-semibold tracking-tight">{name}</span>
      </div>

      <nav className="flex-1 space-y-6 overflow-y-auto p-3">
        <Group label="Platform" items={PLATFORM.map((i) => ({ ...i }))} onNavigate={onNavigate} />
        <Group
          label="Operations"
          items={OPERATIONS.map((i) => ({ ...i }))}
          onNavigate={onNavigate}
        />
        {isAdmin && (
          <Group
            label="Administration"
            items={[{ to: "/settings", label: "Settings", icon: Palette, end: false }]}
            onNavigate={onNavigate}
          />
        )}
      </nav>

      <div className="space-y-2 border-t border-border p-3">
        <div className="flex items-center gap-3 px-2 py-1.5">
          <span
            aria-hidden="true"
            className="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-sm font-medium"
          >
            {initial}
          </span>
          <div className="min-w-0 text-sm leading-tight">
            <p className="font-medium">{isAdmin ? "Administrator" : "Account"}</p>
            <p className="truncate text-xs text-muted-foreground">{user.email}</p>
          </div>
        </div>
        <button
          type="button"
          onClick={signOut}
          className={cn(LINK, "w-full text-muted-foreground")}
        >
          <LogOut />
          <span>Sign out</span>
        </button>
      </div>
    </aside>
  );
}
