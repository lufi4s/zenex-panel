import { Show } from "solid-js";
import type { SiteActivity } from "@/api/types";
import { CheckCircle2, Loader2, XCircle } from "@/components/icons";
import { activityStepText, describeActivity, type ActivityTone } from "@/lib/activity";
import { cn } from "@/lib/utils";

const TONE: Record<ActivityTone, string> = {
  info: "text-foreground",
  success: "text-success",
  danger: "text-destructive",
  neutral: "text-muted-foreground",
};

/**
 * What a website is doing right now: "Backing up · 45%" with a progress bar and the current
 * step, then "Backup complete" or "Backup failed" for a few minutes after it ends. Renders
 * nothing when the website is idle.
 */
export function ActivityStatus(props: { activity: SiteActivity | undefined }) {
  return (
    <Show when={props.activity}>
      {(activity) => {
        const text = () => describeActivity(activity());
        const running = () => activity().status === "running";
        return (
          <div class="mt-1.5 space-y-1" role="status">
            <span
              class={cn("inline-flex items-center gap-1.5 text-xs font-medium", TONE[text().tone])}
            >
              <Show when={text().running}>
                <Loader2 class="size-3 animate-spin" aria-hidden="true" />
              </Show>
              <Show when={activity().status === "succeeded"}>
                <CheckCircle2 class="size-3" aria-hidden="true" />
              </Show>
              <Show when={activity().status === "failed" || activity().status === "dead"}>
                <XCircle class="size-3" aria-hidden="true" />
              </Show>
              {text().text}
            </span>
            <Show when={running()}>
              <div
                role="progressbar"
                aria-label={text().text}
                aria-valuemin={0}
                aria-valuemax={100}
                aria-valuenow={Math.round(activity().percent)}
                class="h-1.5 w-40 max-w-full overflow-hidden rounded-full bg-muted"
              >
                <div
                  class="h-full rounded-full bg-primary transition-[width] duration-500"
                  style={{ width: `${Math.min(100, Math.max(0, activity().percent))}%` }}
                />
              </div>
              <Show when={activity().step}>
                <p class="text-[11px] text-muted-foreground">{activityStepText(activity().step)}</p>
              </Show>
            </Show>
          </div>
        );
      }}
    </Show>
  );
}
