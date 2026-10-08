import { useSyncExternalStore } from "react";
import { AlertTriangle, CheckCircle2, Info, X } from "lucide-react";
import { dismiss, getToasts, subscribe, type ToastTone } from "@/lib/toast";
import { cn } from "@/lib/utils";

const ICON: Record<ToastTone, typeof Info> = {
  success: CheckCircle2,
  error: AlertTriangle,
  info: Info,
};

const TONE_CLASS: Record<ToastTone, string> = {
  success: "border-success/40 text-success",
  error: "border-destructive/40 text-destructive",
  info: "border-border text-foreground",
};

/** Short messages in the corner. Errors are announced to screen readers at once. */
export function Toaster() {
  const items = useSyncExternalStore(subscribe, getToasts, getToasts);
  return (
    <div className="pointer-events-none fixed bottom-4 right-4 z-50 flex w-[min(92vw,360px)] flex-col gap-2">
      {items.map((t) => {
        const Icon = ICON[t.tone];
        return (
          <div
            key={t.id}
            role={t.tone === "error" ? "alert" : "status"}
            className={cn(
              "pointer-events-auto flex items-start gap-3 rounded-lg border bg-card p-3 text-sm shadow-lg",
              TONE_CLASS[t.tone],
            )}
          >
            <Icon className="mt-0.5 size-4 shrink-0" aria-hidden />
            <p className="min-w-0 flex-1 break-words text-foreground">{t.message}</p>
            <button
              type="button"
              onClick={() => dismiss(t.id)}
              className="shrink-0 rounded-sm text-muted-foreground hover:text-foreground"
              aria-label="Dismiss message"
            >
              <X className="size-4" />
            </button>
          </div>
        );
      })}
    </div>
  );
}
