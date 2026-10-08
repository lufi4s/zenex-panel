import { useState } from "react";
import { Plus } from "@/components/icons";
import { useNavigate } from "@/lib/router";
import { useSiteHealth, useSites } from "@/api/queries";
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
  DialogTrigger,
} from "@/components/ui/dialog";
import { CardSkeleton } from "@/components/CardSkeleton";
import { JobProgress } from "@/components/JobProgress";
import { NewWebsiteCard } from "@/components/NewWebsiteCard";
import { PageHeader } from "@/components/PageHeader";
import { badgeTone } from "@/lib/badge";
import { describeError } from "@/api/client";
import { STATE_LABEL, STATE_TONE, timeAgo } from "@/lib/site-status";

function WebsiteTable({ sites, uptime }: { sites: Site[]; uptime: Map<string, string> }) {
  const navigate = useNavigate();
  return (
    <div className="overflow-x-auto rounded-lg border border-border">
      <table className="w-full text-sm">
        <thead className="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
          <tr>
            <th className="px-4 py-2.5 font-medium">Domain</th>
            <th className="px-4 py-2.5 font-medium">Status</th>
            <th className="hidden px-4 py-2.5 font-medium md:table-cell">Uptime (24 h)</th>
            <th className="hidden px-4 py-2.5 font-medium sm:table-cell">PHP</th>
            <th className="hidden px-4 py-2.5 font-medium lg:table-cell">Created</th>
          </tr>
        </thead>
        <tbody>
          {sites.map((site) => (
            <tr
              key={site.id}
              tabIndex={0}
              role="link"
              aria-label={`Open ${site.domain}`}
              onClick={() => navigate(`/websites/${site.id}`)}
              onKeyDown={(e) => {
                if (e.key === "Enter") navigate(`/websites/${site.id}`);
              }}
              className="cursor-pointer border-t border-border hover:bg-muted/40 focus-visible:bg-muted/40 focus-visible:outline-none"
            >
              <td className="px-4 py-3 font-medium">{site.domain}</td>
              <td className="px-4 py-3">
                <Badge variant="outline" className={badgeTone(STATE_TONE[site.state])}>
                  {STATE_LABEL[site.state]}
                </Badge>
              </td>
              <td className="hidden px-4 py-3 tabular-nums text-muted-foreground md:table-cell">
                {uptime.get(site.id) ?? "—"}
              </td>
              <td className="hidden px-4 py-3 text-muted-foreground sm:table-cell">
                PHP {site.php_version}
              </td>
              <td className="hidden px-4 py-3 text-muted-foreground lg:table-cell">
                {timeAgo(site.created_at)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

/** All websites, a create dialog, and a row per site that opens its own page. */
export function WebsitesPage() {
  const sites = useSites();
  const health = useSiteHealth("24h");
  const navigate = useNavigate();
  const [creating, setCreating] = useState(false);
  // The website and build job started from the dialog; set once "Create website" succeeds.
  const [build, setBuild] = useState<{ siteId: string; jobId: string } | null>(null);

  const uptime = new Map(
    (health.data ?? [])
      .filter((h) => h.checks > 0)
      .map((h) => [h.site_id, `${((h.ok_checks / h.checks) * 100).toFixed(1)}%`]),
  );

  const closeDialog = (open: boolean) => {
    setCreating(open);
    if (!open) setBuild(null);
  };

  const newWebsite = (
    <Dialog open={creating} onOpenChange={closeDialog}>
      <DialogTrigger asChild>
        <Button>
          <Plus aria-hidden /> New website
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-lg">
        {build ? (
          <>
            <DialogHeader>
              <DialogTitle>Building website</DialogTitle>
              <DialogDescription>
                Live status of each step. You can close this window; the build keeps running.
              </DialogDescription>
            </DialogHeader>
            <JobProgress
              jobId={build.jobId}
              title="Building your website"
              defaultShowLog
              onDismiss={() => closeDialog(false)}
            />
            <div className="flex justify-end">
              <Button variant="outline" onClick={() => navigate(`/websites/${build.siteId}`)}>
                Open website
              </Button>
            </div>
          </>
        ) : (
          <>
            <DialogHeader>
              <DialogTitle>New website</DialogTitle>
              <DialogDescription>
                Enter a subdomain name and choose its domain. WordPress is installed for you.
              </DialogDescription>
            </DialogHeader>
            <NewWebsiteCard onCreated={(siteId, jobId) => setBuild({ siteId, jobId })} />
          </>
        )}
      </DialogContent>
    </Dialog>
  );

  return (
    <>
      <PageHeader
        title="Websites"
        description="Every WordPress site on this server. Open one to manage files, logs and settings."
        actions={newWebsite}
      />

      {sites.isPending && <CardSkeleton height="h-64" />}
      {sites.isError && (
        <Alert variant="destructive">
          <AlertDescription>{describeError(sites.error)}</AlertDescription>
        </Alert>
      )}
      {sites.data && sites.data.length === 0 && (
        <div className="flex flex-col items-center gap-3 rounded-lg border border-dashed border-border py-16 text-center">
          <p className="font-heading text-lg font-semibold">No websites yet</p>
          <p className="max-w-sm text-sm text-muted-foreground">
            Create a WordPress website on a domain you own. It takes a few minutes to build.
          </p>
          {newWebsite}
        </div>
      )}
      {sites.data && sites.data.length > 0 && <WebsiteTable sites={sites.data} uptime={uptime} />}
    </>
  );
}
