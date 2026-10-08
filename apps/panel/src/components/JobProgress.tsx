import { useEffect, useState, type ReactNode } from "react";
import { CheckCircle2, CircleDashed, Loader2, XCircle } from "lucide-react";
import { useQueryClient } from "@tanstack/react-query";
import { useJob, useJobLogs, useRetryJob } from "@/api/queries";
import { errorMessageFrom } from "@/api/client";
import type { JobLogLine, JobStep, StepStatus } from "@/api/types";
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

const LEVEL_TONE: Record<JobLogLine["level"], string> = {
  info: "text-muted-foreground",
  warn: "text-warning",
  error: "text-destructive",
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

function LogLines({ jobId }: { jobId: string }) {
  const logs = useJobLogs(jobId, true);
  if (logs.isPending) return <p className="text-xs text-muted-foreground">Loading log…</p>;
  if (logs.isError) return <Alert tone="danger">Could not load the log.</Alert>;
  if (!logs.data || logs.data.length === 0)
    return <p className="text-xs text-muted-foreground">No log lines yet.</p>;
  return (
    <div className="max-h-64 overflow-auto rounded-md border border-border bg-background p-3 font-mono text-xs">
      {logs.data.map((line) => (
        <div key={line.id} className="flex gap-3 py-0.5">
          <span className="shrink-0 text-muted-foreground tabular-nums">
            {new Date(line.time).toLocaleTimeString()}
          </span>
          <span className={cn("shrink-0 uppercase", LEVEL_TONE[line.level])}>{line.level}</span>
          <span className="min-w-0 break-words">{line.message}</span>
        </div>
      ))}
    </div>
  );
}

/**
 * Live progress of one background job (building or deleting a website).
 * Polls while the job runs, refreshes the website list when it finishes, offers
 * a retry when a step fails, and can show the full step-by-step log.
 */
export function JobProgress({ jobId, title, onDismiss }: JobProgressProps) {
  const qc = useQueryClient();
  const job = useJob(jobId);
  const retry = useRetryJob();
  const [showLog, setShowLog] = useState(false);
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
        <div className="flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            aria-expanded={showLog}
            onClick={() => setShowLog((v) => !v)}
          >
            {showLog ? "Hide log" : "Show log"}
          </Button>
          {finished && (
            <Button variant="ghost" size="sm" onClick={onDismiss}>
              Dismiss
            </Button>
          )}
        </div>
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
        {showLog && <LogLines jobId={jobId} />}
      </CardContent>
    </Card>
  );
}
