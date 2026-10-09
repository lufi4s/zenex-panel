import { createEffect, For, Show } from "solid-js";
import { useQueryClient } from "@/api/query";
import { useJob, useRetryJob } from "@/api/queries";
import { errorMessageFrom } from "@/api/client";
import type { JobStep, StepStatus } from "@/api/types";
import { CheckCircle2, CircleDashed, Loader2, XCircle } from "@/components/icons";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { stepLabel } from "@/lib/format";
import { cn } from "@/lib/utils";

/** The icon for one build step: a tick, a spinner, a cross, or an empty circle. */
function StepIcon(props: { status: StepStatus }) {
  return (
    <>
      <Show when={props.status === "succeeded"}>
        <CheckCircle2 class="size-4 text-success" />
      </Show>
      <Show when={props.status === "running"}>
        <Loader2 class="size-4 animate-spin text-primary" />
      </Show>
      <Show when={props.status === "failed"}>
        <XCircle class="size-4 text-destructive" />
      </Show>
      <Show
        when={
          props.status !== "succeeded" && props.status !== "running" && props.status !== "failed"
        }
      >
        <CircleDashed class="size-4 text-muted-foreground/60" />
      </Show>
    </>
  );
}

function StepRow(props: { step: JobStep }) {
  const pending = () => props.step.status === "pending" || props.step.status === "skipped";
  return (
    <li class="flex items-start gap-3 py-2">
      <span class="mt-0.5 shrink-0">
        <StepIcon status={props.step.status} />
      </span>
      <div class="min-w-0 flex-1">
        <p class={cn("text-sm", pending() ? "text-muted-foreground" : "text-foreground")}>
          {stepLabel(props.step.name)}
        </p>
        <Show when={props.step.status === "failed" && props.step.error}>
          <p class="mt-0.5 break-words text-xs text-destructive">{props.step.error}</p>
        </Show>
      </div>
    </li>
  );
}

/**
 * Progress of one background job (building or deleting a website): the steps,
 * a retry when a step fails, and a dismiss once it has finished.
 */
export function JobProgress(props: { jobId: string; title: string; onDismiss: () => void }) {
  const qc = useQueryClient();
  // The job query is keyed by the id at mount time, so give a new job its own instance (for example with a keyed Show).
  const job = useJob(props.jobId);
  const retry = useRetryJob();
  const status = () => job.data?.job.status;
  const finished = () => {
    const s = status();
    return s === "succeeded" || s === "failed" || s === "cancelled" || s === "dead";
  };

  createEffect(() => {
    if (finished()) void qc.invalidateQueries({ queryKey: ["sites"] });
  });

  const steps = () => job.data?.steps ?? [];
  const done = () => steps().filter((s) => s.status === "succeeded").length;
  const subtitle = () =>
    status() === "succeeded"
      ? "Your website is ready."
      : status() === "failed"
        ? "The build stopped. Retry to continue from where it failed."
        : steps().length > 0
          ? `${done()} of ${steps().length} steps complete`
          : "Starting…";

  return (
    <div class="space-y-4">
      <div class="space-y-1">
        <p class="font-heading text-base font-semibold">
          {status() === "succeeded" ? "Done" : props.title}
        </p>
        <p class="text-sm text-muted-foreground">{subtitle()}</p>
      </div>

      <Show when={job.isError}>
        <Alert variant="destructive">
          <AlertDescription>
            {errorMessageFrom(null, "Could not load progress. Refresh the page.")}
          </AlertDescription>
        </Alert>
      </Show>

      <Show when={steps().length > 0}>
        <ol class="divide-y divide-border">
          <For each={steps()}>{(step) => <StepRow step={step} />}</For>
        </ol>
      </Show>

      <Show when={status() === "failed" && job.data?.job.error}>
        {(message) => (
          <Alert variant="destructive">
            <AlertDescription>{message()}</AlertDescription>
          </Alert>
        )}
      </Show>
      <Show when={retry.isError}>
        <Alert variant="destructive">
          <AlertDescription>{errorMessageFrom(null, "Could not retry.")}</AlertDescription>
        </Alert>
      </Show>

      <Show when={status() === "failed" || finished()}>
        <div class="flex flex-wrap justify-end gap-2 border-t border-border pt-4">
          <Show when={status() === "failed"}>
            <Button
              variant="outline"
              size="sm"
              disabled={retry.isPending}
              onClick={() => retry.mutate(props.jobId)}
            >
              {retry.isPending ? "Retrying…" : "Retry"}
            </Button>
          </Show>
          <Show when={finished()}>
            <Button variant="ghost" size="sm" onClick={() => props.onDismiss()}>
              Close
            </Button>
          </Show>
        </div>
      </Show>
    </div>
  );
}
