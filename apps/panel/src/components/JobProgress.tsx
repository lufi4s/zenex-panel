import { useEffect, type ReactNode } from "react";
import { CheckCircle2, CircleDashed, Loader2, XCircle } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useJob, useRetryJob } from "@/api/queries";
import { errorMessageFrom } from "@/api/client";
import type { JobStep, StepStatus } from "@/api/types";
import { Alert } from "@/components/ui/badge-alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { stepLabel } from "@/lib/format";
import { cn } from "@/lib/utils";

interface JobProgressProps {
  jobId: string;
  title: string;
  onDismiss: () => void;
}

const ICON: Record<StepStatus, ReactNode> = {
  succeeded: <CheckCircle2 className="text-success" aria-hidden />,
  running: <Loader2 className="animate-spin text-primary" aria-hidden />,
  failed: <XCircle className="text-destructive" aria-hidden />,
  pending: <CircleDashed className="text-muted-foreground" aria-hidden />,
  skipped: <CircleDashed className="text-muted-foreground" aria-hidden />,
};

function StepRow({ step }: { step: JobStep }) {
  return (
    <li className="flex items-start gap-3 py-1.5">
      <span className="mt-0.5 [&_svg]:size-4">{ICON[step.status]}</span>
      <div className="min-w-0 flex-1">
        <span className={cn("text-sm", step.status === "pending" && "text-muted-foreground")}>
          {stepLabel(step.name)}
        </span>
        {step.status === "failed" && step.error && (
          <p className="mt-0.5 break-words text-xs text-destructive">{step.error}</p>
        )}
      </div>
    </li>
  );
}

/**
 * Live progress of one background job (building or deleting a website).
 * Polls while the job runs, refreshes the website list when it finishes, and
 * offers a retry when a step fails.
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

  const heading = status === "succeeded" ? "Done" : status === "failed" ? "Stopped" : title;

  return (
    <Card>
      <CardHeader className="flex-row items-center justify-between gap-3">
        <CardTitle>{heading}</CardTitle>
        {finished && (
          <Button variant="ghost" size="sm" onClick={onDismiss}>
            Dismiss
          </Button>
        )}
      </CardHeader>
      <CardContent className="space-y-3">
        {job.isPending && <p className="text-sm text-muted-foreground">Starting…</p>}
        {job.isError && (
          <Alert tone="danger">
            {errorMessageFrom(null, "Could not load progress. Refresh the page.")}
          </Alert>
        )}
        {job.data && (
          <ol className="divide-y divide-border">
            {job.data.steps.map((step) => (
              <StepRow key={step.name} step={step} />
            ))}
          </ol>
        )}
        {status === "failed" && (
          <div className="space-y-2">
            {job.data?.job.error && <Alert tone="danger">{job.data.job.error}</Alert>}
            <Button
              variant="outline"
              size="sm"
              disabled={retry.isPending}
              onClick={() => retry.mutate(jobId)}
            >
              {retry.isPending ? "Retrying…" : "Retry"}
            </Button>
            {retry.isError && (
              <Alert tone="danger">{errorMessageFrom(null, "Could not retry.")}</Alert>
            )}
          </div>
        )}
        {status === "succeeded" && <Alert tone="success">Finished successfully.</Alert>}
      </CardContent>
    </Card>
  );
}
