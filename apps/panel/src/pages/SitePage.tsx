import { useState } from "react";
import { ExternalLink } from "@/components/icons";
import { Link, useNavigate, useParams } from "@/lib/router";
import {
  useChangePHP,
  useCredentials,
  useDeleteSite,
  usePHPVersions,
  useSite,
  useSiteAction,
  useSiteHealth,
  useSiteLogs,
} from "@/api/queries";
import { ApiError, describeError } from "@/api/client";
import type { Site } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { CardSkeleton } from "@/components/CardSkeleton";
import { FileBrowser } from "@/components/FileManager";
import { JobProgress } from "@/components/JobProgress";
import { PageHeader } from "@/components/PageHeader";
import { Terminal, levelFromText } from "@/components/Terminal";
import { badgeTone } from "@/lib/badge";
import { siteUrl } from "@/lib/format";
import { STATE_LABEL, STATE_TONE } from "@/lib/site-status";

function errorOf(err: unknown, fallback: string): string {
  return err ? describeError(err) || fallback : fallback;
}

// ---------------------------------------------------------------------------
// Overview tab
// ---------------------------------------------------------------------------

function OverviewTab({ site }: { site: Site }) {
  const health = useSiteHealth("24h");
  const action = useSiteAction();
  const phpVersions = usePHPVersions();
  const changePHP = useChangePHP();
  const [phpChoice, setPhpChoice] = useState(site.php_version);

  const mine = health.data?.find((h) => h.site_id === site.id);
  const uptime =
    mine && mine.checks > 0 ? `${((mine.ok_checks / mine.checks) * 100).toFixed(2)}%` : "—";
  const ready = site.state === "ready";
  const suspended = site.state === "suspended";
  const versions = phpVersions.data?.length ? phpVersions.data : [site.php_version];
  const message = action.error ?? changePHP.error;

  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Card className="lg:col-span-2">
        <CardHeader>
          <CardTitle>Status</CardTitle>
          <CardDescription>Last 24 hours of uptime checks, one per minute.</CardDescription>
        </CardHeader>
        <CardContent>
          <dl className="grid grid-cols-2 gap-x-6 gap-y-4 text-sm sm:grid-cols-3">
            <div>
              <dt className="text-muted-foreground">Uptime</dt>
              <dd className="mt-1 text-lg font-semibold tabular-nums">{uptime}</dd>
            </div>
            <div>
              <dt className="text-muted-foreground">Response time</dt>
              <dd className="mt-1 text-lg font-semibold tabular-nums">
                {mine && mine.checks > 0 ? `${mine.avg_latency_ms} ms` : "—"}
              </dd>
            </div>
            <div>
              <dt className="text-muted-foreground">PHP</dt>
              <dd className="mt-1 text-lg font-semibold">{site.php_version}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Actions</CardTitle>
          <CardDescription>Control how this website runs.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-col gap-2">
          {ready && (
            <>
              <Button
                variant="outline"
                disabled={action.isPending}
                onClick={() => action.mutate({ id: site.id, action: "php-restart" })}
              >
                Restart PHP
              </Button>
              <Button
                variant="outline"
                disabled={action.isPending}
                onClick={() => action.mutate({ id: site.id, action: "suspend" })}
              >
                Take offline
              </Button>
            </>
          )}
          {suspended && (
            <Button
              disabled={action.isPending}
              onClick={() => action.mutate({ id: site.id, action: "resume" })}
            >
              Bring online
            </Button>
          )}
          {!ready && !suspended && (
            <p className="text-sm text-muted-foreground">
              Actions appear once the website is live.
            </p>
          )}
        </CardContent>
      </Card>

      {ready && (
        <Card className="lg:col-span-3">
          <CardHeader>
            <CardTitle>PHP version</CardTitle>
            <CardDescription>
              Switching restarts the site&apos;s PHP worker for a moment.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              className="flex flex-wrap items-end gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                if (phpChoice !== site.php_version)
                  changePHP.mutate({ id: site.id, version: phpChoice });
              }}
            >
              <div className="space-y-1.5">
                <Label htmlFor="php-version">Version</Label>
                <select
                  id="php-version"
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
                disabled={changePHP.isPending || phpChoice === site.php_version}
              >
                {changePHP.isPending ? "Changing…" : "Change PHP"}
              </Button>
            </form>
          </CardContent>
        </Card>
      )}

      {Boolean(message) && (
        <Alert variant="destructive" className="lg:col-span-3">
          <AlertDescription>{errorOf(message, "That did not work. Try again.")}</AlertDescription>
        </Alert>
      )}
      {action.isSuccess && !action.isPending && (
        <Alert className="border-success/40 bg-success/5 text-success lg:col-span-3">
          <AlertDescription>Done.</AlertDescription>
        </Alert>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Logs tab
// ---------------------------------------------------------------------------

function LogsTab({ site, active }: { site: Site; active: boolean }) {
  const log = useSiteLogs(site.id, active);

  if (log.isPending) return <CardSkeleton height="h-72" />;
  if (log.isError) {
    return (
      <Alert variant="destructive">
        <AlertDescription>{errorOf(log.error, "Could not read the log.")}</AlertDescription>
      </Alert>
    );
  }
  return (
    <Terminal
      title={`${site.domain} · access log`}
      emptyText="Nothing logged yet."
      maxHeight="65vh"
      lines={(log.data ?? "")
        .split("\n")
        .filter((line) => line.length > 0)
        .map((text, i) => ({ id: i, text, level: levelFromText(text) }))}
    />
  );
}

// ---------------------------------------------------------------------------
// Settings tab: login, and the danger zone
// ---------------------------------------------------------------------------

function WordPressLogin({ site }: { site: Site }) {
  const creds = useCredentials();
  return (
    <Dialog onOpenChange={(open) => open && creds.mutate(site.id)}>
      <DialogTrigger asChild>
        <Button variant="outline">WordPress login</Button>
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
          <Alert variant="destructive">
            <AlertDescription>{errorOf(creds.error, "Could not load the login.")}</AlertDescription>
          </Alert>
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

function DeleteWebsite({ site, onDeleting }: { site: Site; onDeleting: () => void }) {
  const del = useDeleteSite();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");
  const matches = typed.trim().toLowerCase() === site.domain.toLowerCase();

  const confirm = () => {
    if (!matches) return;
    del.mutate(site.id, {
      onSuccess: () => {
        setOpen(false);
        onDeleting();
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
        <Button variant="destructive">Delete website</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete {site.domain}?</DialogTitle>
          <DialogDescription>
            This permanently removes the website&apos;s files, database and login. It cannot be
            undone.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor="confirm-delete">
            Type <span className="font-mono">{site.domain}</span> to confirm
          </Label>
          <Input
            id="confirm-delete"
            autoComplete="off"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
          />
        </div>
        {del.isError && (
          <Alert variant="destructive">
            <AlertDescription>
              {errorOf(del.error, "Could not delete the website.")}
            </AlertDescription>
          </Alert>
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

function SettingsTab({ site, onDeleting }: { site: Site; onDeleting: () => void }) {
  return (
    <div className="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>WordPress</CardTitle>
          <CardDescription>Sign in to the admin dashboard of this website.</CardDescription>
        </CardHeader>
        <CardContent className="flex flex-wrap gap-2">
          {site.state === "ready" && (
            <Button variant="outline" asChild>
              <a href={`${siteUrl(site.domain)}/wp-admin/`} target="_blank" rel="noopener">
                <ExternalLink aria-hidden /> Open admin
              </a>
            </Button>
          )}
          <WordPressLogin site={site} />
        </CardContent>
      </Card>

      <Card className="border-destructive/40">
        <CardHeader>
          <CardTitle className="text-destructive">Danger zone</CardTitle>
          <CardDescription>
            Deleting a website removes its files, database and admin login.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DeleteWebsite site={site} onDeleting={onDeleting} />
        </CardContent>
      </Card>
    </div>
  );
}

// ---------------------------------------------------------------------------
// The page
// ---------------------------------------------------------------------------

/** One website: its status, build progress, and tabs for overview, files, logs and settings. */
export function SitePage() {
  const { id = "" } = useParams();
  const navigate = useNavigate();
  const detail = useSite(id);
  const [tab, setTab] = useState("overview");
  const [dismissedJob, setDismissedJob] = useState<string | null>(null);

  if (detail.isPending) {
    return (
      <>
        <PageHeader title="Website" />
        <CardSkeleton height="h-72" />
      </>
    );
  }
  if (detail.isError) {
    const notFound = detail.error instanceof ApiError && detail.error.status === 404;
    return (
      <>
        <PageHeader title={notFound ? "Website not found" : "Website"} />
        <Alert variant="destructive">
          <AlertDescription>
            {notFound ? "This website does not exist or was deleted." : describeError(detail.error)}
          </AlertDescription>
        </Alert>
        <Button variant="outline" className="self-start" asChild>
          <Link to="/websites">Back to websites</Link>
        </Button>
      </>
    );
  }

  if (!detail.data) return null;
  const { site, job } = detail.data;
  const showJob =
    job &&
    job.id !== dismissedJob &&
    (job.status === "queued" || job.status === "running" || job.status === "failed");
  const canBrowseFiles = site.state === "ready" || site.state === "suspended";

  return (
    <>
      <PageHeader
        title={site.domain}
        description={`Created ${new Date(site.created_at).toLocaleDateString()} · WordPress on PHP ${site.php_version}`}
        actions={
          <>
            <Badge variant="outline" className={badgeTone(STATE_TONE[site.state])}>
              {STATE_LABEL[site.state]}
            </Badge>
            {site.state === "ready" && (
              <Button variant="outline" asChild>
                <a href={siteUrl(site.domain)} target="_blank" rel="noopener">
                  <ExternalLink aria-hidden /> Visit site
                </a>
              </Button>
            )}
          </>
        }
      />

      {showJob && (
        <JobProgress
          jobId={job.id}
          title={job.status === "failed" ? "Build stopped" : "Building your website"}
          onDismiss={() => setDismissedJob(job.id)}
        />
      )}

      <Tabs value={tab} onValueChange={setTab}>
        <TabsList>
          <TabsTrigger value="overview">Overview</TabsTrigger>
          {canBrowseFiles && <TabsTrigger value="files">Files</TabsTrigger>}
          <TabsTrigger value="logs">Logs</TabsTrigger>
          <TabsTrigger value="settings">Settings</TabsTrigger>
        </TabsList>

        <TabsContent value="overview">
          <OverviewTab site={site} />
        </TabsContent>
        {canBrowseFiles && (
          <TabsContent value="files">
            <FileBrowser site={site} />
          </TabsContent>
        )}
        <TabsContent value="logs">
          <LogsTab site={site} active={tab === "logs"} />
        </TabsContent>
        <TabsContent value="settings">
          <SettingsTab site={site} onDeleting={() => navigate("/websites")} />
        </TabsContent>
      </Tabs>
    </>
  );
}
