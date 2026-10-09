import { createSignal, For, onCleanup } from "solid-js";
import { Dynamic } from "solid-js/web";
import { AlertTriangle, CheckCircle2, Info, X } from "@/components/icons";
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
  const [items, setItems] = createSignal(getToasts());
  const unsubscribe = subscribe(() => setItems(getToasts()));
  onCleanup(unsubscribe);

  return (
    <div class="pointer-events-none fixed bottom-4 right-4 z-[60] flex w-[min(92vw,360px)] flex-col gap-2">
      <For each={items()}>
        {(t) => (
          <div
            role={t.tone === "error" ? "alert" : "status"}
            class={cn(
              "pointer-events-auto flex items-start gap-3 rounded-lg border bg-card p-3 text-sm shadow-lg",
              TONE_CLASS[t.tone],
            )}
          >
            <Dynamic component={ICON[t.tone]} class="mt-0.5 size-4 shrink-0" aria-hidden="true" />
            <p class="min-w-0 flex-1 break-words text-foreground">{t.message}</p>
            <button
              type="button"
              onClick={() => dismiss(t.id)}
              class="shrink-0 rounded-sm text-muted-foreground hover:text-foreground"
              aria-label="Dismiss message"
            >
              <X class="size-4" />
            </button>
          </div>
        )}
      </For>
    </div>
  );
}
