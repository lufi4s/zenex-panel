import { useEffect } from "react";
import { useQueryClient } from "@/api/query";
import { useJob, useRetryJob } from "@/api/queries";
import { errorMessageFrom } from "@/api/client";
import type { JobStep, StepStatus } from "@/api/types";
import { CheckCircle2, CircleDashed, Loader2, XCircle } from "@/components/icons";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { stepLabel } from "@/lib/format";
import { cn } from "@/lib/utils";

interface JobProgressProps {
  jobId: string;
  title: string;
  onDismiss: () => void;
}

/** The icon for one build step: a tick, a spinner, a cross, or an empty circle. */
function StepIcon({ status }: { status: StepStatus }) {
  if (status === "succeeded") return <CheckCircle2 className="size-4 text-success" />;
  if (status === "running") return <Loader2 className="size-4 animate-spin text-primary" />;
  if (status === "failed") return <XCircle className="size-4 text-destructive" />;
  return <CircleDashed className="size-4 text-muted-foreground/60" />;
}

function StepRow({ step }: { step: JobStep }) {
  const pending = step.status === "pending" || step.status === "skipped";
  return (
    <li className="flex items-start gap-3 py-2">
      <span className="mt-0.5 shrink-0">
        <StepIcon status={step.status} />
      </span>
      <div className="min-w-0 flex-1">
        <p className={cn("text-sm", pending ? "text-muted-foreground" : "text-foreground")}>
          {stepLabel(step.name)}
        </p>
        {step.status === "failed" && step.error && (
          <p className="mt-0.5 break-words text-xs text-destructive">{step.error}</p>
        )}
      </div>
    </li>
  );
}

/**
 * Progress of one background job (building or deleting a website): the steps,
 * a retry when a step fails, and a dismiss once it has finished.
 */
export function JobProgress({ jobId, title, onDismiss }: JobProgressProps) {
  const qc = useQueryClient();
  const job = useJob(jobId);
  const retry = useRetryJob();
  const status = job.data?.job.status;
  const finished =
    status === "succeeded" || status === "failed" || status === "cancelled" || status === "dead";

  useEffect(() => {
    if (finished) qc.invalidateQueries({ queryKey: ["sites"] });
  }, [finished, qc]);

  const steps = job.data?.steps ?? [];
  const done = steps.filter((s) => s.status === "succeeded").length;
  const subtitle =
    status === "succeeded"
      ? "Your website is ready."
      : status === "failed"
        ? "The build stopped. Retry to continue from where it failed."
        : steps.length > 0
          ? `${done} of ${steps.length} steps complete`
          : "Starting…";

  return (
    <div className="space-y-4">
      <div className="space-y-1">
        <p className="font-heading text-base font-semibold">
          {status === "succeeded" ? "Done" : title}
        </p>
        <p className="text-sm text-muted-foreground">{subtitle}</p>
      </div>

      {job.isError && (
        <Alert variant="destructive">
          <AlertDescription>
            {errorMessageFrom(null, "Could not load progress. Refresh the page.")}
          </AlertDescription>
        </Alert>
      )}

      {steps.length > 0 && (
        <ol className="divide-y divide-border">
          {steps.map((step) => (
            <StepRow key={step.name} step={step} />
          ))}
        </ol>
      )}

      {status === "failed" && job.data?.job.error && (
        <Alert variant="destructive">
          <AlertDescription>{job.data.job.error}</AlertDescription>
        </Alert>
      )}
      {retry.isError && (
        <Alert variant="destructive">
          <AlertDescription>{errorMessageFrom(null, "Could not retry.")}</AlertDescription>
        </Alert>
      )}

      {(status === "failed" || finished) && (
        <div className="flex flex-wrap justify-end gap-2 border-t border-border pt-4">
          {status === "failed" && (
            <Button
              variant="outline"
              size="sm"
              disabled={retry.isPending}
              onClick={() => retry.mutate(jobId)}
            >
              {retry.isPending ? "Retrying…" : "Retry"}
            </Button>
          )}
          {finished && (
            <Button variant="ghost" size="sm" onClick={onDismiss}>
              Close
            </Button>
          )}
        </div>
      )}
    </div>
  );
}
