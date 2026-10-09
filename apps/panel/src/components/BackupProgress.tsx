import { createEffect, createSignal, For, Show } from "solid-js";
import { useQueryClient } from "@/api/query";
import { useBackupProgress } from "@/api/queries";
import type { BackupStep, BackupStepName, BackupStepStatus } from "@/api/types";
import { Check, CircleDashed, Loader2, XCircle } from "@/components/icons";
import { cn } from "@/lib/utils";

const STEP_LABELS: Record<BackupStepName, string> = {
  archive: "Archiving files and database",
  upload: "Uploading to the remote server",
  record: "Saving the backup record",
};

/** The icon for one backup step: a tick, a spinner, a cross, or an empty dashed circle. */
function StepIcon(props: { status: BackupStepStatus }) {
  return (
    <>
      <Show when={props.status === "succeeded"}>
        <Check aria-hidden="true" class="size-4 text-success" />
      </Show>
      <Show when={props.status === "running"}>
        <Loader2 aria-hidden="true" class="size-4 animate-spin text-primary" />
      </Show>
      <Show when={props.status === "failed"}>
        <XCircle aria-hidden="true" class="size-4 text-destructive" />
      </Show>
      <Show when={props.status === "pending"}>
        <CircleDashed aria-hidden="true" class="size-4 text-muted-foreground/60" />
      </Show>
    </>
  );
}

function StepRow(props: { step: BackupStep }) {
  const pending = () => props.step.status === "pending";
  return (
    <li class="flex items-center gap-3 py-1.5">
      <span class="shrink-0">
        <StepIcon status={props.step.status} />
      </span>
      <span class={cn("text-sm", pending() ? "text-muted-foreground" : "text-foreground")}>
        {STEP_LABELS[props.step.name] ?? props.step.name}
      </span>
    </li>
  );
}

/**
 * Progress of the website's latest backup: a bar, the percent, the status and the steps.
 * Polls while the backup runs. Renders nothing when there is no backup job, or when the last
 * backup succeeded before this page was opened.
 */
export function BackupProgress(props: { siteId: string }) {
  const qc = useQueryClient();
  const progress = useBackupProgress(props.siteId);
  const [seenActive, setSeenActive] = createSignal(false);
  let wasActive = false;

  const status = () => progress.data?.status;
  const percent = () => Math.min(100, Math.max(0, Math.round(progress.data?.percent ?? 0)));

  // When a backup this page watched finishes, refresh the backup list.
  createEffect(() => {
    const s = status();
    if (s === "queued" || s === "running") {
      wasActive = true;
      setSeenActive(true);
      return;
    }
    if ((s === "succeeded" || s === "failed") && wasActive) {
      wasActive = false;
      void qc.invalidateQueries({ queryKey: ["backups", props.siteId] });
    }
  });

  const visible = () => {
    const data = progress.data;
    if (!data || !data.job_id || !data.status) return false;
    return data.status === "succeeded" ? seenActive() : true;
  };

  return (
    <Show when={visible()}>
      <div class="space-y-3 rounded-lg border border-border p-4">
        <div class="flex items-center justify-between gap-3">
          <p
            class={cn(
              "text-sm font-medium",
              status() === "failed" ? "text-destructive" : "text-foreground",
            )}
          >
            {status() === "succeeded"
              ? "Backup complete"
              : status() === "failed"
                ? "Backup failed"
                : "Backing up..."}
          </p>
          <span class="text-sm tabular-nums text-muted-foreground">{percent()}%</span>
        </div>

        <div
          role="progressbar"
          aria-label="Backup progress"
          aria-valuenow={percent()}
          aria-valuemin={0}
          aria-valuemax={100}
          class="h-2 w-full overflow-hidden rounded-full bg-muted"
        >
          <div
            class="h-full rounded-full bg-primary transition-[width] duration-500"
            style={{ width: `${percent()}%` }}
          />
        </div>

        <Show when={(progress.data?.steps ?? []).length > 0}>
          <ol class="space-y-0.5">
            <For each={progress.data?.steps ?? []}>{(step) => <StepRow step={step} />}</For>
          </ol>
        </Show>
      </div>
    </Show>
  );
}
