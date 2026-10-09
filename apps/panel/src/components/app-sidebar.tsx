import { For, Show } from "solid-js";
import {
  Activity,
  Globe,
  Layers,
  LayoutDashboard,
  LogOut,
  Settings,
  Server,
} from "@/components/icons";
import { useBranding, useLogout } from "@/api/queries";
import type { User } from "@/api/types";
import { brandAssetVersion } from "@/lib/brand";
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

type NavItem = { to: string; label: string; icon: typeof Server; end: boolean };

function Group(props: { label: string; items: NavItem[]; onNavigate: () => void }) {
  return (
    <div class="space-y-1">
      <p class="px-3 pb-1 text-xs font-medium uppercase tracking-wider text-muted-foreground">
        {props.label}
      </p>
      <For each={props.items}>
        {(item) => (
          <NavLink
            to={item.to}
            end={item.end}
            onClick={props.onNavigate}
            class={cn(
              LINK,
              "data-[active]:bg-muted data-[active]:font-medium data-[active]:text-foreground",
              "text-muted-foreground",
            )}
          >
            <item.icon />
            <span>{item.label}</span>
          </NavLink>
        )}
      </For>
    </div>
  );
}

/** The left navigation. On small screens it slides in as a drawer. */
export function AppSidebar(props: { user: User; open: boolean; onNavigate: () => void }) {
  const branding = useBranding();
  const logout = useLogout();
  const navigate = useNavigate();
  const name = () => branding.data?.name ?? "Zenex Panel";
  const isAdmin = () => props.user.roles.includes("administrator");
  const initial = () => props.user.email.charAt(0).toUpperCase();

  const signOut = () => {
    logout.mutate(undefined, { onSettled: () => navigate("/login", { replace: true }) });
  };

  return (
    <aside
      aria-label="Main navigation"
      class={cn(
        "fixed inset-y-0 left-0 z-40 flex w-64 flex-col border-r border-border bg-card transition-[transform,visibility] duration-200",
        "lg:sticky lg:top-0 lg:h-dvh lg:shrink-0 lg:translate-x-0",
        // A closed drawer is also hidden, so keyboard focus cannot reach its links.
        props.open ? "translate-x-0" : "-translate-x-full max-lg:invisible",
      )}
    >
      <div class="flex h-14 items-center gap-2.5 border-b border-border px-4">
        <Show
          when={branding.data?.has_logo}
          fallback={
            <span
              aria-hidden="true"
              class="flex size-8 shrink-0 items-center justify-center rounded-md bg-primary text-sm font-semibold text-primary-foreground"
            >
              {name().trim().charAt(0).toUpperCase() || "Z"}
            </span>
          }
        >
          <img
            src={`/api/v1/branding/logo?v=${brandAssetVersion()}`}
            alt=""
            class="size-8 shrink-0 rounded-md object-contain"
          />
        </Show>
        <span class="truncate font-heading text-base font-semibold tracking-tight">{name()}</span>
      </div>

      <nav class="flex-1 space-y-6 overflow-y-auto p-3">
        <Group label="Platform" items={PLATFORM} onNavigate={props.onNavigate} />
        <Group label="Operations" items={OPERATIONS} onNavigate={props.onNavigate} />
        {isAdmin() && (
          <Group
            label="Administration"
            items={[{ to: "/settings", label: "Settings", icon: Settings, end: false }]}
            onNavigate={props.onNavigate}
          />
        )}
      </nav>

      <div class="space-y-2 border-t border-border p-3">
        <div class="flex items-center gap-3 px-2 py-1.5">
          <span
            aria-hidden="true"
            class="flex size-8 shrink-0 items-center justify-center rounded-full bg-muted text-sm font-medium"
          >
            {initial()}
          </span>
          <div class="min-w-0 text-sm leading-tight">
            <p class="font-medium">{isAdmin() ? "Administrator" : "Account"}</p>
            <p class="truncate text-xs text-muted-foreground">{props.user.email}</p>
          </div>
        </div>
        <button type="button" onClick={signOut} class={cn(LINK, "w-full text-muted-foreground")}>
          <LogOut />
          <span>Sign out</span>
        </button>
      </div>
    </aside>
  );
}
