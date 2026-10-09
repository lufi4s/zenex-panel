import { createSignal, For, Show } from "solid-js";
import { useQueryClient } from "@/api/query";
import { describeError } from "@/api/client";
import { useRemoteBackups, useRestoreRemote, useSettingsBackups, useSites } from "@/api/queries";
import type { RemoteBackup } from "@/api/types";
import { CardSkeleton } from "@/components/CardSkeleton";
import { JobProgress } from "@/components/JobProgress";
import { RotateCcw } from "@/components/icons";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Select } from "@/components/ui/select";

/** Groups backups by website domain. The list arrives newest first, so the newest group comes first. */
function groupByDomain(list: RemoteBackup[]): Array<{ domain: string; backups: RemoteBackup[] }> {
  const groups = new Map<string, RemoteBackup[]>();
  for (const backup of list) {
    const group = groups.get(backup.domain);
    if (group) group.push(backup);
    else groups.set(backup.domain, [backup]);
  }
  return [...groups].map(([domain, backups]) => ({ domain, backups }));
}

function megabytes(bytes: number): string {
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`;
}

/**
 * Backups found on the SFTP backup server. Every panel that saves to the same server sees
 * them, so a new panel connected to that server lists the websites it can restore here.
 * Shown only when the backup destination is SFTP.
 */
export function RemoteBackupsCard() {
  const settings = useSettingsBackups();
  return (
    <Show when={settings.data?.destination.type === "sftp"}>
      <RemoteBackupsBody />
    </Show>
  );
}

function RemoteBackupsBody() {
  const qc = useQueryClient();
  const list = useRemoteBackups();
  const sites = useSites();
  const restore = useRestoreRemote();
  // The backup chosen for restore, the website it will be restored into, and the started job.
  const [chosen, setChosen] = createSignal<RemoteBackup | null>(null);
  const [siteId, setSiteId] = createSignal("");
  const [job, setJob] = createSignal<string | null>(null);

  const groups = () => groupByDomain(list.data ?? []);
  const readySites = () => (sites.data ?? []).filter((s) => s.state === "ready");
  const sameDomainSite = () => {
    const backup = chosen();
    return backup ? readySites().find((s) => s.domain === backup.domain) : undefined;
  };

  const choose = (backup: RemoteBackup) => {
    setChosen(backup);
    // Pick the website that already has this domain, so the usual case needs one click.
    const match = readySites().find((s) => s.domain === backup.domain);
    setSiteId(match?.id ?? readySites()[0]?.id ?? "");
  };

  const confirm = () => {
    const backup = chosen();
    const id = siteId();
    if (!backup || !id) return;
    restore.mutate(
      { siteId: id, path: backup.path },
      {
        onSuccess: (res) => {
          setChosen(null);
          setJob(res.job_id);
        },
      },
    );
  };

  return (
    <Card>
      <CardHeader>
        <div class="flex flex-wrap items-start justify-between gap-3">
          <div class="space-y-1.5">
            <CardTitle>Backups on the backup server</CardTitle>
            <CardDescription>
              Backups saved to your SFTP server, by this panel or any other Zenex panel that uses
              it. Restore one on a website here to bring it back, for example after moving to a new
              server.
            </CardDescription>
          </div>
          <Button
            variant="outline"
            size="sm"
            disabled={list.isPending}
            onClick={() => void list.refetch()}
          >
            Refresh
          </Button>
        </div>
      </CardHeader>
      <CardContent class="space-y-4">
        <Show when={list.isError}>
          <Alert variant="destructive">
            <AlertDescription>{describeError(list.error)}</AlertDescription>
          </Alert>
        </Show>
        <Show when={restore.isError}>
          <Alert variant="destructive">
            <AlertDescription>{describeError(restore.error)}</AlertDescription>
          </Alert>
        </Show>

        <Show when={list.isPending && !list.isError}>
          <CardSkeleton height="h-24" />
        </Show>

        <Show when={list.data && list.data.length === 0}>
          <p class="text-sm text-muted-foreground">
            No backups found on the backup server yet. Backups appear here after they are uploaded.
          </p>
        </Show>

        <For each={groups()}>
          {(group) => (
            <section class="overflow-hidden rounded-lg border border-border">
              <header class="flex flex-wrap items-center justify-between gap-2 bg-muted/40 px-4 py-2.5">
                <h3 class="font-medium text-foreground">{group.domain}</h3>
                <p class="text-xs text-muted-foreground">
                  {group.backups.length === 1 ? "1 backup" : `${group.backups.length} backups`}
                  {" · latest "}
                  {new Date(group.backups[0].created_at).toLocaleString()}
                </p>
              </header>
              <ul class="divide-y divide-border">
                <For each={group.backups}>
                  {(backup) => (
                    <li class="flex flex-wrap items-center justify-between gap-3 px-4 py-2.5">
                      <div class="min-w-0">
                        <p class="text-sm tabular-nums text-foreground">
                          {new Date(backup.created_at).toLocaleString()}
                        </p>
                        <p class="text-xs tabular-nums text-muted-foreground">
                          {megabytes(backup.size_bytes)}
                        </p>
                      </div>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={restore.isPending || readySites().length === 0}
                        onClick={() => choose(backup)}
                      >
                        <RotateCcw aria-hidden="true" />
                        Restore
                      </Button>
                    </li>
                  )}
                </For>
              </ul>
            </section>
          )}
        </For>

        <Show when={job()} keyed>
          {(jobId) => (
            <JobProgress
              jobId={jobId}
              title="Restoring backup"
              retryable={false}
              onDismiss={() => {
                setJob(null);
                void qc.invalidateQueries({ queryKey: ["backups"] });
              }}
            />
          )}
        </Show>
      </CardContent>

      <Dialog
        open={chosen() !== null}
        onOpenChange={(open) => {
          if (!open) setChosen(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Restore {chosen()?.domain}?</DialogTitle>
            <DialogDescription>
              Choose the website on this panel to restore into. Its files and database are replaced
              with the backup from {chosen() ? new Date(chosen()!.created_at).toLocaleString() : ""}
              . A copy of that website is saved first.
            </DialogDescription>
          </DialogHeader>

          <div class="space-y-1.5">
            <Label for="restore-target">Website on this panel</Label>
            <Select
              id="restore-target"
              value={siteId()}
              onChange={(e) => setSiteId(e.currentTarget.value)}
            >
              <For each={readySites()}>
                {(site) => (
                  <option value={site.id} selected={site.id === siteId()}>
                    {site.domain}
                    {site.domain === chosen()?.domain ? " (same domain)" : ""}
                  </option>
                )}
              </For>
            </Select>
            <Show when={!sameDomainSite()}>
              <p class="text-xs text-muted-foreground">
                This panel has no website for {chosen()?.domain}. Create it under Websites first, or
                restore into another website.
              </p>
            </Show>
          </div>

          <div class="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setChosen(null)}>
              Cancel
            </Button>
            <Button disabled={!siteId() || restore.isPending} onClick={confirm}>
              {restore.isPending ? "Starting…" : "Restore backup"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
