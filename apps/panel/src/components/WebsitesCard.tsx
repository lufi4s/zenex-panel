import { useState } from "react";
import { ChevronDown, ExternalLink } from "lucide-react";
import {
  useChangePHP,
  useCredentials,
  useDeleteSite,
  usePHPVersions,
  useSiteAction,
  useSiteHealth,
  useSiteLog,
  useSites,
} from "@/api/queries";
import { ApiError, errorMessageFrom } from "@/api/client";
import type { Site, SiteHealth, SiteState } from "@/api/types";
import { Alert, Badge } from "@/components/ui/badge-alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input, Label } from "@/components/ui/input";
import { siteUrl } from "@/lib/format";
import { Terminal, levelFromText } from "@/components/Terminal";
import { FileManager } from "@/components/FileManager";

const STATE_LABEL: Record<SiteState, string> = {
  ready: "Live",
  provisioning: "Building",
  suspended: "Suspended",
  failed: "Failed",
  deleting: "Deleting",
  deleted: "Deleted",
};

const STATE_TONE: Record<SiteState, "success" | "warning" | "danger" | "neutral"> = {
  ready: "success",
  provisioning: "warning",
  suspended: "neutral",
  failed: "danger",
  deleting: "warning",
  deleted: "neutral",
};

function errorOf(err: unknown, fallback: string): string {
  return err instanceof ApiError ? err.message : errorMessageFrom(null, fallback);
}

// ---------------------------------------------------------------------------
// Dialogs
// ---------------------------------------------------------------------------

function LoginDialog({ site }: { site: Site }) {
  const creds = useCredentials();
  return (
    <Dialog onOpenChange={(open) => open && creds.mutate(site.id)}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          WordPress login
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>WordPress login</DialogTitle>
          <DialogDescription>
            Use these details to sign in to the dashboard of {site.domain}.
          </DialogDescription>
        </DialogHeader>
        {creds.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}
        {creds.isError && (
          <Alert tone="danger">{errorOf(creds.error, "Could not load the login.")}</Alert>
        )}
        {creds.data && (
          <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
            <dt className="text-muted-foreground">Dashboard</dt>
            <dd className="break-all">
              <a
                className="text-primary underline-offset-4 hover:underline"
                href={creds.data.url}
                target="_blank"
                rel="noopener"
              >
                {creds.data.url}
              </a>
            </dd>
            <dt className="text-muted-foreground">Username</dt>
            <dd className="font-mono">{creds.data.username}</dd>
            <dt className="text-muted-foreground">Password</dt>
            <dd className="break-all font-mono">{creds.data.password}</dd>
          </dl>
        )}
      </DialogContent>
    </Dialog>
  );
}

function LogDialog({ site }: { site: Site }) {
  const log = useSiteLog();
  return (
    <Dialog onOpenChange={(open) => open && log.mutate(site.id)}>
      <DialogTrigger asChild>
        <Button variant="outline" size="sm">
          Site log
        </Button>
      </DialogTrigger>
      <DialogContent className="max-h-[80vh] w-[min(92vw,760px)] overflow-hidden">
        <DialogHeader>
          <DialogTitle>Site log</DialogTitle>
          <DialogDescription>Recent requests to {site.domain}.</DialogDescription>
        </DialogHeader>
        {log.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}
        {log.isError && (
          <Alert tone="danger">{errorOf(log.error, "Could not read the log.")}</Alert>
        )}
        {log.data !== undefined && (
          <Terminal
            title={`${site.domain} · site log`}
            emptyText="Nothing logged yet."
            maxHeight="55vh"
            lines={(log.data ?? "")
              .split("\n")
              .filter((line) => line.length > 0)
              .map((text, i) => ({ id: i, text, level: levelFromText(text) }))}
          />
        )}
      </DialogContent>
    </Dialog>
  );
}

function DeleteDialog({
  site,
  onStarted,
}: {
  site: Site;
  onStarted: (jobId: string, title: string) => void;
}) {
  const del = useDeleteSite();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");
  const matches = typed.trim().toLowerCase() === site.domain.toLowerCase();

  const confirm = () => {
    if (!matches) return;
    del.mutate(site.id, {
      onSuccess: (result) => {
        setOpen(false);
        setTyped("");
        onStarted(result.job_id, `Deleting ${site.domain}`);
      },
    });
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) setTyped("");
      }}
    >
      <DialogTrigger asChild>
        <Button variant="destructive" size="sm">
          Delete
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete {site.domain}?</DialogTitle>
          <DialogDescription>
            This permanently removes the website's files, database and login. It cannot be undone.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor={`confirm-${site.id}`}>
            Type <span className="font-mono">{site.domain}</span> to confirm
          </Label>
          <Input
            id={`confirm-${site.id}`}
            autoComplete="off"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
          />
        </div>
        {del.isError && (
          <Alert tone="danger">{errorOf(del.error, "Could not delete the website.")}</Alert>
        )}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button variant="destructive" disabled={!matches || del.isPending} onClick={confirm}>
            {del.isPending ? "Deleting…" : "Delete permanently"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

// ---------------------------------------------------------------------------
// One website
// ---------------------------------------------------------------------------

function WebsiteRow({
  site,
  onStarted,
  health,
}: {
  site: Site;
  onStarted: (jobId: string, title: string) => void;
  health?: SiteHealth;
}) {
  const [expanded, setExpanded] = useState(false);
  const action = useSiteAction();
  const phpVersions = usePHPVersions();
  const changePHP = useChangePHP();
  const [phpChoice, setPhpChoice] = useState(site.php_version);

  const ready = site.state === "ready";
  const busy = site.state === "provisioning" || site.state === "deleting";
  const versions = phpVersions.data?.length ? phpVersions.data : [site.php_version];
  const message = action.error ?? changePHP.error;

  const run = (name: "suspend" | "resume" | "php-restart") =>
    action.mutate({ id: site.id, action: name });

  return (
    <li className="py-3 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center gap-3">
        <div className="min-w-0 flex-1">
          {ready ? (
            <a
              href={siteUrl(site.domain)}
              target="_blank"
              rel="noopener"
              className="inline-flex max-w-full items-center gap-1 truncate font-medium text-primary underline-offset-4 hover:underline"
            >
              <span className="truncate">{site.domain}</span>
              <ExternalLink className="size-3.5 shrink-0" aria-hidden />
            </a>
          ) : (
            <span className="font-medium">{site.domain}</span>
          )}
        </div>
        <Badge tone={STATE_TONE[site.state]}>
          {busy && <span className="size-1.5 animate-pulse rounded-full bg-current" aria-hidden />}
          {STATE_LABEL[site.state]}
        </Badge>
        {health && health.checks > 0 && (
          <span
            className="text-xs tabular-nums text-muted-foreground"
            title="Last 24 hours, one check per minute"
          >
            {((health.ok_checks / health.checks) * 100).toFixed(1)}% up · {health.avg_latency_ms} ms
          </span>
        )}
        <Button
          variant="ghost"
          size="sm"
          aria-expanded={expanded}
          aria-controls={`site-${site.id}`}
          onClick={() => setExpanded((v) => !v)}
        >
          Manage
          <ChevronDown
            className={`transition-transform ${expanded ? "rotate-180" : ""}`}
            aria-hidden
          />
        </Button>
      </div>

      {expanded && (
        <div
          id={`site-${site.id}`}
          className="mt-3 space-y-4 rounded-md border border-border bg-background p-4"
        >
          {ready && (
            <div className="flex flex-wrap gap-2">
              <Button variant="outline" size="sm" asChild>
                <a href={`${siteUrl(site.domain)}/wp-admin/`} target="_blank" rel="noopener">
                  <ExternalLink aria-hidden /> WordPress admin
                </a>
              </Button>
              <LoginDialog site={site} />
              <FileManager site={site} />
              <LogDialog site={site} />
              <Button
                variant="outline"
                size="sm"
                disabled={action.isPending}
                onClick={() => run("php-restart")}
              >
                Restart PHP
              </Button>
              <Button
                variant="outline"
                size="sm"
                disabled={action.isPending}
                onClick={() => run("suspend")}
              >
                Take offline
              </Button>
            </div>
          )}
          {site.state === "suspended" && (
            <div className="flex flex-wrap gap-2">
              <Button size="sm" disabled={action.isPending} onClick={() => run("resume")}>
                Bring online
              </Button>
              <LogDialog site={site} />
            </div>
          )}

          {ready && (
            <form
              className="flex flex-wrap items-end gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                if (phpChoice !== site.php_version)
                  changePHP.mutate({ id: site.id, version: phpChoice });
              }}
            >
              <div className="space-y-1.5">
                <Label htmlFor={`php-${site.id}`}>PHP version</Label>
                <select
                  id={`php-${site.id}`}
                  className="flex h-10 rounded-md border border-input bg-card px-3 text-base sm:h-9 sm:text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/30"
                  value={phpChoice}
                  onChange={(e) => setPhpChoice(e.target.value)}
                >
                  {versions.map((v) => (
                    <option key={v} value={v}>
                      PHP {v}
                    </option>
                  ))}
                </select>
              </div>
              <Button
                type="submit"
                variant="outline"
                size="sm"
                disabled={changePHP.isPending || phpChoice === site.php_version}
              >
                {changePHP.isPending ? "Changing…" : "Change PHP"}
              </Button>
            </form>
          )}

          {message && (
            <Alert tone="danger">{errorOf(message, "That did not work. Try again.")}</Alert>
          )}
          {action.isSuccess && !action.isPending && <Alert tone="success">Done.</Alert>}

          <div className="border-t border-border pt-4">
            <p className="mb-2 text-xs text-muted-foreground">Danger zone</p>
            <DeleteDialog site={site} onStarted={onStarted} />
          </div>
        </div>
      )}
    </li>
  );
}

// ---------------------------------------------------------------------------
// The list
// ---------------------------------------------------------------------------

export function WebsitesCard({ onStarted }: { onStarted: (jobId: string, title: string) => void }) {
  const sites = useSites();
  const health = useSiteHealth("24h");
  const healthById = new Map((health.data ?? []).map((h) => [h.site_id, h]));

  return (
    <Card>
      <CardHeader>
        <CardTitle>Your websites</CardTitle>
        <CardDescription>Everything you have built on this server.</CardDescription>
      </CardHeader>
      <CardContent>
        {sites.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}
        {sites.isError && <Alert tone="danger">Could not load your websites.</Alert>}
        {sites.data && sites.data.length === 0 && (
          <p className="text-sm text-muted-foreground">
            No websites yet. Create your first one above.
          </p>
        )}
        {sites.data && sites.data.length > 0 && (
          <ul className="divide-y divide-border">
            {sites.data.map((site) => (
              <WebsiteRow
                key={site.id}
                site={site}
                onStarted={onStarted}
                health={healthById.get(site.id)}
              />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
