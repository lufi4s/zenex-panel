import { useEffect, useRef, useState } from "react";
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

function Item({ n, onRead }: { n: Notification; onRead: (id: number) => void }) {
  const Icon = ICON[n.level];
  const unread = !n.read_at;
  return (
    <li>
      <button
        type="button"
        onClick={() => unread && onRead(n.id)}
        className={cn(
          "flex w-full items-start gap-3 rounded-md px-3 py-2.5 text-left hover:bg-muted",
          unread && "bg-primary/5",
        )}
      >
        <Icon className={cn("mt-0.5 size-4 shrink-0", ICON_TONE[n.level])} aria-hidden />
        <div className="min-w-0 flex-1">
          <div className="flex items-center gap-2">
            <span className={cn("truncate text-sm", unread ? "font-semibold" : "font-medium")}>
              {n.title}
            </span>
            {unread && (
              <span className="size-1.5 shrink-0 rounded-full bg-primary" aria-label="Unread" />
            )}
          </div>
          {n.body && <p className="mt-0.5 break-words text-xs text-muted-foreground">{n.body}</p>}
          <p className="mt-1 text-[11px] text-muted-foreground">{ago(n.created_at)}</p>
        </div>
      </button>
    </li>
  );
}

/** Header bell with an unread count and a panel of recent notifications. */
export function NotificationBell() {
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);
  const feed = useNotifications();
  const mark = useMarkNotificationsRead();
  const unread = feed.data?.unread ?? 0;

  useEffect(() => {
    if (!open) return;
    const close = (event: MouseEvent | KeyboardEvent) => {
      if (event instanceof KeyboardEvent) {
        if (event.key === "Escape") setOpen(false);
        return;
      }
      if (root.current && !root.current.contains(event.target as Node)) setOpen(false);
    };
    document.addEventListener("mousedown", close);
    document.addEventListener("keydown", close);
    return () => {
      document.removeEventListener("mousedown", close);
      document.removeEventListener("keydown", close);
    };
  }, [open]);

  return (
    <div ref={root} className="relative">
      <Button
        variant="ghost"
        size="icon"
        aria-label={unread > 0 ? `Notifications, ${unread} unread` : "Notifications"}
        aria-expanded={open}
        onClick={() => setOpen((v) => !v)}
        className="relative"
      >
        <Bell aria-hidden />
        {unread > 0 && (
          <span className="absolute -right-0.5 -top-0.5 flex min-w-4 items-center justify-center rounded-full bg-destructive px-1 text-[10px] font-semibold leading-4 text-destructive-foreground">
            {unread > 9 ? "9+" : unread}
          </span>
        )}
      </Button>

      {open && (
        <div className="fixed inset-x-3 top-16 z-30 overflow-hidden rounded-lg border border-border bg-card shadow-xl md:absolute md:inset-x-auto md:right-0 md:top-auto md:mt-2 md:w-[380px]">
          <div className="flex items-center justify-between border-b border-border px-3 py-2">
            <span className="text-sm font-semibold">Notifications</span>
            <Button
              variant="ghost"
              size="sm"
              disabled={unread === 0 || mark.isPending}
              onClick={() => mark.mutate({ all: true })}
            >
              <CheckCheck aria-hidden />
              Mark all read
            </Button>
          </div>
          <div className="max-h-[60vh] overflow-y-auto p-1.5">
            {feed.isPending && <p className="p-4 text-sm text-muted-foreground">Loading…</p>}
            {feed.isError && (
              <p className="p-4 text-sm text-destructive">Could not load notifications.</p>
            )}
            {feed.data && feed.data.items.length === 0 && (
              <p className="p-4 text-sm text-muted-foreground">
                You're all caught up. Nothing new.
              </p>
            )}
            {feed.data && feed.data.items.length > 0 && (
              <ul className="space-y-0.5">
                {feed.data.items.map((n) => (
                  <Item key={n.id} n={n} onRead={(id) => mark.mutate({ ids: [id] })} />
                ))}
              </ul>
            )}
          </div>
        </div>
      )}
    </div>
  );
}
