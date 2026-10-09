import { createEffect, createSignal, For, onCleanup, Show } from "solid-js";
import { Dynamic } from "solid-js/web";
import { Bell, CheckCheck, CheckCircle2, Info, AlertTriangle, XCircle } from "@/components/icons";
import { useMarkNotificationsRead, useNotifications } from "@/api/queries";
import type { Notification, NotificationLevel } from "@/api/types";
import { Button } from "@/components/ui/button";
import { cn } from "@/lib/utils";

const ICON: Record<NotificationLevel, typeof Info> = {
  info: Info,
  success: CheckCircle2,
  warning: AlertTriangle,
  error: XCircle,
};

const ICON_TONE: Record<NotificationLevel, string> = {
  info: "text-muted-foreground",
  success: "text-success",
  warning: "text-warning",
  error: "text-destructive",
};

function ago(iso: string): string {
  const seconds = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (seconds < 60) return "just now";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric" });
}

function Item(props: { n: Notification; onRead: (id: number) => void }) {
  const unread = () => !props.n.read_at;
  return (
    <li>
      <button
        type="button"
        onClick={() => unread() && props.onRead(props.n.id)}
        class={cn(
          "flex w-full items-start gap-3 rounded-md px-3 py-2.5 text-left hover:bg-muted",
          unread() && "bg-primary/5",
        )}
      >
        <Dynamic
          component={ICON[props.n.level]}
          class={cn("mt-0.5 size-4 shrink-0", ICON_TONE[props.n.level])}
          aria-hidden="true"
        />
        <div class="min-w-0 flex-1">
          <div class="flex items-center gap-2">
            <span class={cn("truncate text-sm", unread() ? "font-semibold" : "font-medium")}>
              {props.n.title}
            </span>
            <Show when={unread()}>
              <span class="size-1.5 shrink-0 rounded-full bg-primary" aria-label="Unread" />
            </Show>
          </div>
          <Show when={props.n.body}>
            <p class="mt-0.5 break-words text-xs text-muted-foreground">{props.n.body}</p>
          </Show>
          <p class="mt-1 text-[11px] text-muted-foreground">{ago(props.n.created_at)}</p>
        </div>
      </button>
    </li>
  );
}

/** Header bell with an unread count and a panel of recent notifications. */
export function NotificationBell() {
  const [open, setOpen] = createSignal(false);
  let root: HTMLDivElement | undefined;
  const feed = useNotifications();
  const mark = useMarkNotificationsRead();
  const unread = () => feed.data?.unread ?? 0;

  createEffect(() => {
    if (!open()) return;
    const close = (event: MouseEvent | KeyboardEvent) => {
      if (event instanceof KeyboardEvent) {
        if (event.key === "Escape") setOpen(false);
        return;
      }
      if (root && !root.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", close);
    onCleanup(() => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", close);
    });
  });

  return (
    <div ref={(el) => (root = el)} class="relative">
      <Button
        variant="ghost"
        size="icon"
        aria-label={unread() > 0 ? `Notifications, ${unread()} unread` : "Notifications"}
        aria-expanded={open()}
        onClick={() => setOpen((v) => !v)}
        class="relative"
      >
        <Bell aria-hidden="true" />
        <Show when={unread() > 0}>
          <span class="absolute -right-0.5 -top-0.5 flex min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[10px] font-semibold leading-4 text-destructive-foreground">
            {unread() > 9 ? "9+" : unread()}
          </span>
        </Show>
      </Button>

      <Show when={open()}>
        <div class="fixed inset-x-3 top-16 z-30 overflow-hidden rounded-lg border border-border bg-card shadow-xl md:absolute md:inset-x-auto md:right-0 md:top-auto md:mt-2 md:w-[380px]">
          <div class="flex items-center justify-between border-b border-border px-3 py-2">
            <span class="text-sm font-semibold">Notifications</span>
            <Button
              variant="ghost"
              size="sm"
              disabled={unread() === 0 || mark.isPending}
              onClick={() => mark.mutate({ all: true })}
            >
              <CheckCheck aria-hidden="true" />
              Mark all read
            </Button>
          </div>
          <div class="max-h-[60vh] overflow-y-auto p-1.5">
            <Show when={feed.isPending}>
              <p class="p-4 text-sm text-muted-foreground">Loading…</p>
            </Show>
            <Show when={feed.isError}>
              <p class="p-4 text-sm text-destructive">Could not load notifications.</p>
            </Show>
            <Show when={feed.data && feed.data.items.length === 0}>
              <p class="p-4 text-sm text-muted-foreground">You're all caught up. Nothing new.</p>
            </Show>
            <Show when={feed.data && feed.data.items.length > 0 ? feed.data : undefined}>
              {(page) => (
                <ul class="space-y-0.5">
                  <For each={page().items}>
                    {(n) => <Item n={n} onRead={(id) => mark.mutate({ ids: [id] })} />}
                  </For>
                </ul>
              )}
            </Show>
          </div>
        </div>
      </Show>
    </div>
  );
}
