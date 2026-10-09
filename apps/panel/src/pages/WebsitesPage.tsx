import { createMemo, createSignal, For, Show } from "solid-js";
import { Plus } from "@/components/icons";
import { useNavigate } from "@/lib/router";
import { useBackupAll, useMe, useSiteHealth, useSites } from "@/api/queries";
import type { Site } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { CardSkeleton } from "@/components/CardSkeleton";
import { JobProgress } from "@/components/JobProgress";
import { NewWebsiteCard } from "@/components/NewWebsiteCard";
import { MigrateFromCpanel } from "@/components/MigrateFromCpanel";
import { PageHeader } from "@/components/PageHeader";
import { badgeTone } from "@/lib/badge";
import { describeError } from "@/api/client";
import { STATE_LABEL, STATE_TONE, timeAgo } from "@/lib/site-status";

function WebsiteTable(props: { sites: Site[]; uptime: Map<string, string> }) {
  const navigate = useNavigate();
  return (
    <div class="overflow-x-auto rounded-lg border border-border">
      <table class="w-full text-sm">
        <thead class="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
          <tr>
            <th class="px-4 py-2.5 font-medium">Domain</th>
            <th class="px-4 py-2.5 font-medium">Status</th>
            <th class="hidden px-4 py-2.5 font-medium md:table-cell">Uptime (24 h)</th>
            <th class="hidden px-4 py-2.5 font-medium sm:table-cell">PHP</th>
            <th class="hidden px-4 py-2.5 font-medium lg:table-cell">Created</th>
          </tr>
        </thead>
        <tbody>
          <For each={props.sites}>
            {(site) => (
              <tr
                tabIndex={0}
                role="link"
                aria-label={`Open ${site.domain}`}
                onClick={() => navigate(`/websites/${site.id}`)}
                onKeyDown={(e) => {
                  if (e.key === "Enter") navigate(`/websites/${site.id}`);
                }}
                class="cursor-pointer border-t border-border hover:bg-muted/40 focus-visible:bg-muted/40 focus-visible:outline-none"
              >
                <td class="px-4 py-3 font-medium">{site.domain}</td>
                <td class="px-4 py-3">
                  <Badge variant="outline" class={badgeTone(STATE_TONE[site.state])}>
                    {STATE_LABEL[site.state]}
                  </Badge>
                </td>
                <td class="hidden px-4 py-3 tabular-nums text-muted-foreground md:table-cell">
                  {props.uptime.get(site.id) ?? "—"}
                </td>
                <td class="hidden px-4 py-3 text-muted-foreground sm:table-cell">
                  PHP {site.php_version}
                </td>
                <td class="hidden px-4 py-3 text-muted-foreground lg:table-cell">
                  {timeAgo(site.created_at)}
                </td>
              </tr>
            )}
          </For>
        </tbody>
      </table>
    </div>
  );
}

/** All websites, a create dialog, and a row per site that opens its own page. */
export function WebsitesPage() {
  const sites = useSites();
  const health = useSiteHealth("24h");
  const backupAll = useBackupAll();
  const me = useMe();
  const isAdmin = () => me.data?.roles.includes("administrator") ?? false;
  const navigate = useNavigate();
  const [creating, setCreating] = createSignal(false);
  // The website and build job started from the dialog; set once "Create website" succeeds.
  const [build, setBuild] = createSignal<{ siteId: string; jobId: string } | null>(null);

  const uptime = createMemo(
    () =>
      new Map(
        (health.data ?? [])
          .filter((h) => h.checks > 0)
          .map((h): [string, string] => [
            h.site_id,
            `${((h.ok_checks / h.checks) * 100).toFixed(1)}%`,
          ]),
      ),
  );

  const closeDialog = (open: boolean) => {
    setCreating(open);
    if (!open) setBuild(null);
  };

  const newWebsiteButton = () => (
    <Button onClick={() => setCreating(true)}>
      <Plus aria-hidden /> New website
    </Button>
  );

  // One dialog for the whole page; the buttons only open it. Two dialogs on one signal would
  // open on top of each other.
  const newWebsiteDialog = (
    <Dialog open={creating()} onOpenChange={closeDialog}>
      <DialogContent class="max-h-[90vh] overflow-y-auto sm:max-w-xl">
        <Show
          when={build()?.jobId}
          keyed
          fallback={
            <>
              <DialogHeader>
                <DialogTitle>New website</DialogTitle>
                <DialogDescription>
                  Enter a subdomain name and choose its domain. WordPress is installed for you.
                </DialogDescription>
              </DialogHeader>
              <NewWebsiteCard onCreated={(siteId, jobId) => setBuild({ siteId, jobId })} />
            </>
          }
        >
          {(jobId) => (
            <>
              <DialogHeader>
                <DialogTitle>Building website</DialogTitle>
                <DialogDescription>
                  Live status of each step. You can close this window; the build keeps running.
                </DialogDescription>
              </DialogHeader>
              <JobProgress
                jobId={jobId}
                title="Building your website"
                onDismiss={() => closeDialog(false)}
              />
              <div class="flex justify-end">
                <Button
                  variant="outline"
                  onClick={() => navigate(`/websites/${build()?.siteId ?? ""}`)}
                >
                  Open website
                </Button>
              </div>
            </>
          )}
        </Show>
      </DialogContent>
    </Dialog>
  );

  return (
    <>
      <PageHeader
        title="Websites"
        description="Every WordPress site on this server. Open one to manage files, logs and settings."
        actions={
          <div class="flex flex-wrap gap-2">
            <Button
              variant="outline"
              disabled={backupAll.isPending || !sites.data?.length}
              onClick={() => backupAll.mutate()}
            >
              {backupAll.isPending ? "Starting…" : "Back up all websites"}
            </Button>
            <Show when={isAdmin()}>
              <MigrateFromCpanel />
            </Show>
            {newWebsiteButton()}
          </div>
        }
      />

      <Show when={sites.isPending}>
        <CardSkeleton height="h-64" />
      </Show>
      <Show when={sites.isError}>
        <Alert variant="destructive">
          <AlertDescription>{describeError(sites.error)}</AlertDescription>
        </Alert>
      </Show>
      <Show when={sites.data && sites.data.length === 0}>
        <div class="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border py-16 text-center">
          <p class="font-heading text-lg font-semibold">No websites yet</p>
          <p class="max-w-sm text-sm text-muted-foreground">
            Create a WordPress website on a domain you own. It takes a few minutes to build.
          </p>
          {newWebsiteButton()}
        </div>
      </Show>
      <Show when={(sites.data?.length ?? 0) > 0}>
        <WebsiteTable sites={sites.data ?? []} uptime={uptime()} />
      </Show>
      {newWebsiteDialog}
    </>
  );
}
