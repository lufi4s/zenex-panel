import { createSignal, For, Show } from "solid-js";
import { describeError } from "@/api/client";
import {
  useDeleteQuarantined,
  useQuarantineFile,
  useRestoreQuarantined,
  useSecurity,
  useStartScan,
} from "@/api/queries";
import type { FindingSeverity, ScanFinding, ScanReport, Site } from "@/api/types";
import { CardSkeleton } from "@/components/CardSkeleton";
import { CheckCircle2, Loader2, RotateCcw, ShieldCheck, Trash2 } from "@/components/icons";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { badgeTone } from "@/lib/badge";
import { formatBytes } from "@/lib/format";

const SEVERITY_LABEL: Record<FindingSeverity, string> = {
  high: "High",
  medium: "Medium",
  low: "Low",
};
const SEVERITY_TONE: Record<FindingSeverity, string> = {
  high: "danger",
  medium: "warning",
  low: "neutral",
};

/**
 * Findings whose file belongs to WordPress or a plugin. Moving such a file away would break
 * the website, so the fix is to reinstall that part, not to quarantine the file.
 */
const REPAIR_RULES = new Set(["core-modified", "plugin-modified"]);

/** A one-line summary of a scan: how many problems, and how serious. */
export function summarizeReport(report: ScanReport): {
  text: string;
  tone: "success" | "warning" | "danger";
} {
  const count = report.findings.length;
  if (count === 0) return { text: "No problems found", tone: "success" };
  const high = report.findings.filter((f) => f.severity === "high").length;
  const noun = count === 1 ? "issue" : "issues";
  return {
    text: high > 0 ? `${count} ${noun} · ${high} serious` : `${count} ${noun}`,
    tone: high > 0 ? "danger" : "warning",
  };
}

function seconds(ms: number): string {
  return ms < 1000 ? "under a second" : `${Math.round(ms / 1000)} s`;
}

/** Malware scanner for one website: scan now, review findings, quarantine files. */
export function SecurityTab(props: { site: Site }) {
  const security = useSecurity(props.site.id);
  const scan = useStartScan(props.site.id);
  const quarantine = useQuarantineFile(props.site.id);
  const restore = useRestoreQuarantined(props.site.id);
  const purge = useDeleteQuarantined(props.site.id);
  const [error, setError] = createSignal("");
  const [confirmDelete, setConfirmDelete] = createSignal<string | null>(null);
  const [busy, setBusy] = createSignal<string | null>(null);

  // A file that was moved to quarantine is no longer a problem of the website.
  const visibleReport = (): ScanReport | null => {
    const data = security.data;
    if (!data?.report) return null;
    const moved = new Set(data.quarantine.map((item) => item.path));
    return { ...data.report, findings: data.report.findings.filter((f) => !moved.has(f.path)) };
  };

  const running = () => security.data?.running === true || scan.isPending;
  const fail = (e: unknown) => {
    setBusy(null);
    setError(describeError(e));
  };
  const act = (key: string, run: (done: () => void) => void) => {
    setError("");
    setBusy(key);
    run(() => setBusy(null));
  };

  const startScan = () => {
    setError("");
    scan.mutate(undefined, { onError: fail });
  };
  const quarantineFile = (finding: ScanFinding) =>
    act(finding.path, (done) =>
      quarantine.mutate(
        { path: finding.path, rule: finding.rule },
        { onSuccess: done, onError: fail },
      ),
    );

  return (
    <div class="space-y-6">
      <Card>
        <CardHeader>
          <div class="flex flex-wrap items-start justify-between gap-3">
            <div>
              <CardTitle class="flex items-center gap-2">
                <ShieldCheck class="size-5" aria-hidden="true" /> Malware scan
              </CardTitle>
              <CardDescription>
                Looks through the website's files for hidden backdoors, web shells and code that
                does not belong, and compares WordPress and plugin files with the official ones.
              </CardDescription>
            </div>
            <Button onClick={startScan} disabled={running()}>
              <Show when={running()} fallback={<ShieldCheck aria-hidden="true" />}>
                <Loader2 class="animate-spin" aria-hidden="true" />
              </Show>
              {running() ? "Scanning…" : "Scan now"}
            </Button>
          </div>
        </CardHeader>
        <CardContent class="space-y-4">
          <Show when={error()}>
            <Alert variant="destructive">
              <AlertDescription>{error()}</AlertDescription>
            </Alert>
          </Show>
          <Show when={security.data?.last_error}>
            <Alert variant="destructive">
              <AlertDescription>{security.data?.last_error}</AlertDescription>
            </Alert>
          </Show>

          <Show when={!security.isPending} fallback={<CardSkeleton height="h-24" />}>
            <Show
              when={!security.isError}
              fallback={
                <Alert variant="destructive">
                  <AlertDescription>{describeError(security.error)}</AlertDescription>
                </Alert>
              }
            >
              <Show
                when={visibleReport()}
                fallback={
                  <p class="text-sm text-muted-foreground" role="status">
                    {running()
                      ? "Scanning the website's files. This takes up to a few minutes."
                      : "This website has not been scanned yet."}
                  </p>
                }
              >
                {(report) => <ReportBody report={report()} running={running()} />}
              </Show>
              <Show when={visibleReport()}>
                {(report) => (
                  <FindingList
                    findings={report().findings}
                    busy={busy()}
                    onQuarantine={quarantineFile}
                  />
                )}
              </Show>
            </Show>
          </Show>
        </CardContent>
      </Card>

      <Show when={(security.data?.quarantine.length ?? 0) > 0}>
        <Card>
          <CardHeader>
            <CardTitle>Quarantine</CardTitle>
            <CardDescription>
              These files were moved out of the website and can no longer run. Restore a file if it
              was flagged by mistake, or delete it for good.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <ul class="divide-y rounded-md border">
              <For each={security.data?.quarantine}>
                {(item) => (
                  <li class="flex flex-wrap items-center justify-between gap-3 p-3">
                    <div class="min-w-0">
                      <p class="font-mono text-sm break-all">{item.path}</p>
                      <p class="text-xs text-muted-foreground">
                        {new Date(item.quarantined_at).toLocaleString()} · {formatBytes(item.size)}
                      </p>
                    </div>
                    <div class="flex gap-2">
                      <Button
                        variant="outline"
                        size="sm"
                        disabled={busy() === item.id}
                        onClick={() =>
                          act(item.id, (done) =>
                            restore.mutate(item.id, { onSuccess: done, onError: fail }),
                          )
                        }
                      >
                        <RotateCcw aria-hidden="true" /> Restore
                      </Button>
                      <Show
                        when={confirmDelete() === item.id}
                        fallback={
                          <Button
                            variant="outline"
                            size="sm"
                            disabled={busy() === item.id}
                            onClick={() => setConfirmDelete(item.id)}
                          >
                            <Trash2 aria-hidden="true" /> Delete
                          </Button>
                        }
                      >
                        <Button
                          variant="destructive"
                          size="sm"
                          disabled={busy() === item.id}
                          onClick={() =>
                            act(item.id, (done) =>
                              purge.mutate(item.id, {
                                onSuccess: () => {
                                  setConfirmDelete(null);
                                  done();
                                },
                                onError: fail,
                              }),
                            )
                          }
                        >
                          Delete for good
                        </Button>
                        <Button variant="ghost" size="sm" onClick={() => setConfirmDelete(null)}>
                          Cancel
                        </Button>
                      </Show>
                    </div>
                  </li>
                )}
              </For>
            </ul>
          </CardContent>
        </Card>
      </Show>
    </div>
  );
}

function ReportBody(props: { report: ScanReport; running: boolean }) {
  const summary = () => summarizeReport(props.report);
  return (
    <div class="space-y-3">
      <div class="flex flex-wrap items-center gap-2">
        <Badge variant="outline" class={badgeTone(summary().tone)}>
          <Show when={summary().tone === "success"}>
            <CheckCircle2 class="size-3" aria-hidden="true" />
          </Show>
          {summary().text}
        </Badge>
        <span class="text-xs text-muted-foreground">
          {props.running ? "Previous scan: " : "Last scan: "}
          {new Date(props.report.scanned_at).toLocaleString()} ·{" "}
          {props.report.files_scanned.toLocaleString()} files · {seconds(props.report.duration_ms)}
        </span>
      </div>
      <Show when={props.report.checksum_note}>
        <Alert>
          <AlertDescription>{props.report.checksum_note}</AlertDescription>
        </Alert>
      </Show>
      <Show when={props.report.truncated}>
        <Alert>
          <AlertDescription>
            The scan stopped early because the website has a very large number of files or findings.
            Remove what is listed and scan again.
          </AlertDescription>
        </Alert>
      </Show>
    </div>
  );
}

function FindingList(props: {
  findings: ScanFinding[];
  busy: string | null;
  onQuarantine: (finding: ScanFinding) => void;
}) {
  return (
    <Show when={props.findings.length > 0}>
      <ul class="divide-y rounded-md border">
        <For each={props.findings}>
          {(finding) => (
            <li class="flex flex-wrap items-start justify-between gap-3 p-3">
              <div class="min-w-0 space-y-1">
                <div class="flex flex-wrap items-center gap-2">
                  <Badge variant="outline" class={badgeTone(SEVERITY_TONE[finding.severity])}>
                    {SEVERITY_LABEL[finding.severity]}
                  </Badge>
                  <span class="font-mono text-sm break-all">
                    {finding.path}
                    <Show when={finding.line}>
                      <span class="text-muted-foreground">:{finding.line}</span>
                    </Show>
                  </span>
                </div>
                <p class="text-sm text-muted-foreground">{finding.message}</p>
                <Show when={REPAIR_RULES.has(finding.rule)}>
                  <p class="text-xs text-muted-foreground">
                    This file belongs to WordPress or a plugin. Update or reinstall it to repair it.
                  </p>
                </Show>
              </div>
              <Show when={!REPAIR_RULES.has(finding.rule)}>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={props.busy === finding.path}
                  onClick={() => props.onQuarantine(finding)}
                >
                  Quarantine
                </Button>
              </Show>
            </li>
          )}
        </For>
      </ul>
    </Show>
  );
}
