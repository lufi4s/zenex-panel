import { createSignal, For, Show } from "solid-js";
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
import { Button, buttonClasses } from "@/components/ui/button";
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

function OverviewTab(props: { site: Site }) {
  const health = useSiteHealth("24h");
  const action = useSiteAction();
  const phpVersions = usePHPVersions();
  const changePHP = useChangePHP();
  const [phpChoice, setPhpChoice] = createSignal(props.site.php_version);

  const mine = () => health.data?.find((h) => h.site_id === props.site.id);
  const uptime = () => {
    const m = mine();
    return m && m.checks > 0 ? `${((m.ok_checks / m.checks) * 100).toFixed(2)}%` : "—";
  };
  const ready = () => props.site.state === "ready";
  const suspended = () => props.site.state === "suspended";
  const versions = () => (phpVersions.data?.length ? phpVersions.data : [props.site.php_version]);
  const message = () => action.error ?? changePHP.error;

  return (
    <div class="grid gap-4 lg:grid-cols-3">
      <Card class="lg:col-span-2">
        <CardHeader>
          <CardTitle>Status</CardTitle>
          <CardDescription>Last 24 hours of uptime checks, one per minute.</CardDescription>
        </CardHeader>
        <CardContent>
          <dl class="grid grid-cols-2 gap-x-6 gap-y-4 text-sm sm:grid-cols-3">
            <div>
              <dt class="text-muted-foreground">Uptime</dt>
              <dd class="mt-1 text-lg font-semibold tabular-nums">{uptime()}</dd>
            </div>
            <div>
              <dt class="text-muted-foreground">Response time</dt>
              <dd class="mt-1 text-lg font-semibold tabular-nums">
                {(mine()?.checks ?? 0) > 0 ? `${mine()?.avg_latency_ms} ms` : "—"}
              </dd>
            </div>
            <div>
              <dt class="text-muted-foreground">PHP</dt>
              <dd class="mt-1 text-lg font-semibold">{props.site.php_version}</dd>
            </div>
          </dl>
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle>Actions</CardTitle>
          <CardDescription>Control how this website runs.</CardDescription>
        </CardHeader>
        <CardContent class="flex flex-col gap-2">
          <Show when={ready()}>
            <Button
              variant="outline"
              disabled={action.isPending}
              onClick={() => action.mutate({ id: props.site.id, action: "php-restart" })}
            >
              Restart PHP
            </Button>
            <Button
              variant="outline"
              disabled={action.isPending}
              onClick={() => action.mutate({ id: props.site.id, action: "suspend" })}
            >
              Take offline
            </Button>
          </Show>
          <Show when={suspended()}>
            <Button
              disabled={action.isPending}
              onClick={() => action.mutate({ id: props.site.id, action: "resume" })}
            >
              Bring online
            </Button>
          </Show>
          <Show when={!ready() && !suspended()}>
            <p class="text-sm text-muted-foreground">Actions appear once the website is live.</p>
          </Show>
        </CardContent>
      </Card>

      <Show when={ready()}>
        <Card class="lg:col-span-3">
          <CardHeader>
            <CardTitle>PHP version</CardTitle>
            <CardDescription>
              Switching restarts the site&apos;s PHP worker for a moment.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <form
              class="flex flex-wrap items-end gap-2"
              onSubmit={(e) => {
                e.preventDefault();
                if (phpChoice() !== props.site.php_version)
                  changePHP.mutate({ id: props.site.id, version: phpChoice() });
              }}
            >
              <div class="space-y-1.5">
                <Label for="php-version">Version</Label>
                <select
                  id="php-version"
                  class="flex h-10 rounded-md border border-input bg-card px-3 text-base sm:h-9 sm:text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/30"
                  onChange={(e) => setPhpChoice(e.currentTarget.value)}
                >
                  <For each={versions()}>
                    {(v) => (
                      <option value={v} selected={v === phpChoice()}>
                        PHP {v}
                      </option>
                    )}
                  </For>
                </select>
              </div>
              <Button
                type="submit"
                variant="outline"
                disabled={changePHP.isPending || phpChoice() === props.site.php_version}
              >
                {changePHP.isPending ? "Changing…" : "Change PHP"}
              </Button>
            </form>
          </CardContent>
        </Card>
      </Show>

      <Show when={Boolean(message())}>
        <Alert variant="destructive" class="lg:col-span-3">
          <AlertDescription>{errorOf(message(), "That did not work. Try again.")}</AlertDescription>
        </Alert>
      </Show>
      <Show when={action.isSuccess && !action.isPending}>
        <Alert class="border-success/40 bg-success/5 text-success lg:col-span-3">
          <AlertDescription>Done.</AlertDescription>
        </Alert>
      </Show>
    </div>
  );
}

// ---------------------------------------------------------------------------
// Logs tab (mounted only while its tab is open, so the log is read on demand)
// ---------------------------------------------------------------------------

function LogsTab(props: { site: Site }) {
  const log = useSiteLogs(props.site.id, true);

  return (
    <Show when={!log.isPending} fallback={<CardSkeleton height="h-72" />}>
      <Show
        when={!log.isError}
        fallback={
          <Alert variant="destructive">
            <AlertDescription>{errorOf(log.error, "Could not read the log.")}</AlertDescription>
          </Alert>
        }
      >
        <Terminal
          title={`${props.site.domain} · access log`}
          emptyText="Nothing logged yet."
          maxHeight="65vh"
          lines={(log.data ?? "")
            .split("\n")
            .filter((line) => line.length > 0)
            .map((text, i) => ({ id: i, text, level: levelFromText(text) }))}
        />
      </Show>
    </Show>
  );
}

// ---------------------------------------------------------------------------
// Settings tab: login, and the danger zone
// ---------------------------------------------------------------------------

function WordPressLogin(props: { site: Site }) {
  const creds = useCredentials();
  return (
    <Dialog
      onOpenChange={(open) => {
        if (open) creds.mutate(props.site.id);
      }}
    >
      <DialogTrigger>
        <Button variant="outline">WordPress login</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>WordPress login</DialogTitle>
          <DialogDescription>
            Use these details to sign in to the dashboard of {props.site.domain}.
          </DialogDescription>
        </DialogHeader>
        <Show when={creds.isPending}>
          <p class="text-sm text-muted-foreground">Loading…</p>
        </Show>
        <Show when={creds.isError}>
          <Alert variant="destructive">
            <AlertDescription>{errorOf(creds.error, "Could not load the login.")}</AlertDescription>
          </Alert>
        </Show>
        <Show when={creds.data}>
          {(c) => (
            <dl class="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
              <dt class="text-muted-foreground">Dashboard</dt>
              <dd class="break-all">
                <a
                  class="text-primary underline-offset-4 hover:underline"
                  href={c().url}
                  target="_blank"
                  rel="noopener"
                >
                  {c().url}
                </a>
              </dd>
              <dt class="text-muted-foreground">Username</dt>
              <dd class="font-mono">{c().username}</dd>
              <dt class="text-muted-foreground">Password</dt>
              <dd class="break-all font-mono">{c().password}</dd>
            </dl>
          )}
        </Show>
      </DialogContent>
    </Dialog>
  );
}

function DeleteWebsite(props: { site: Site; onDeleting: () => void }) {
  const del = useDeleteSite();
  const [open, setOpen] = createSignal(false);
  const [typed, setTyped] = createSignal("");
  const matches = () => typed().trim().toLowerCase() === props.site.domain.toLowerCase();

  const confirm = () => {
    if (!matches()) return;
    del.mutate(props.site.id, {
      onSuccess: () => {
        setOpen(false);
        props.onDeleting();
      },
    });
  };

  return (
    <Dialog
      open={open()}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) setTyped("");
      }}
    >
      <DialogTrigger>
        <Button variant="destructive">Delete website</Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete {props.site.domain}?</DialogTitle>
          <DialogDescription>
            This permanently removes the website&apos;s files, database and login. It cannot be
            undone.
          </DialogDescription>
        </DialogHeader>
        <div class="space-y-1.5">
          <Label for="confirm-delete">
            Type <span class="font-mono">{props.site.domain}</span> to confirm
          </Label>
          <Input
            id="confirm-delete"
            autocomplete="off"
            value={typed()}
            onInput={(e) => setTyped(e.currentTarget.value)}
          />
        </div>
        <Show when={del.isError}>
          <Alert variant="destructive">
            <AlertDescription>
              {errorOf(del.error, "Could not delete the website.")}
            </AlertDescription>
          </Alert>
        </Show>
        <div class="flex justify-end gap-2">
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button variant="destructive" disabled={!matches() || del.isPending} onClick={confirm}>
            {del.isPending ? "Deleting…" : "Delete permanently"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function SettingsTab(props: { site: Site; onDeleting: () => void }) {
  return (
    <div class="grid gap-4 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>WordPress</CardTitle>
          <CardDescription>Sign in to the admin dashboard of this website.</CardDescription>
        </CardHeader>
        <CardContent class="flex flex-wrap gap-2">
          <Show when={props.site.state === "ready"}>
            <a
              href={`${siteUrl(props.site.domain)}/wp-admin/`}
              target="_blank"
              rel="noopener"
              class={buttonClasses("outline")}
            >
              <ExternalLink aria-hidden /> Open admin
            </a>
          </Show>
          <WordPressLogin site={props.site} />
        </CardContent>
      </Card>

      <Card class="border-destructive/40">
        <CardHeader>
          <CardTitle class="text-destructive">Danger zone</CardTitle>
          <CardDescription>
            Deleting a website removes its files, database and admin login.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <DeleteWebsite site={props.site} onDeleting={props.onDeleting} />
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
  const params = useParams();
  const navigate = useNavigate();
  const detail = useSite(params.id ?? "");
  const [tab, setTab] = createSignal("overview");
  const [dismissedJob, setDismissedJob] = createSignal<string | null>(null);

  const notFound = () => detail.error instanceof ApiError && detail.error.status === 404;
  const activeJob = () => {
    const job = detail.data?.job;
    if (!job || job.id === dismissedJob()) return undefined;
    return job.status === "queued" || job.status === "running" || job.status === "failed"
      ? job
      : undefined;
  };

  return (
    <Show
      when={!detail.isPending}
      fallback={
        <>
          <PageHeader title="Website" />
          <CardSkeleton height="h-72" />
        </>
      }
    >
      <Show
        when={!detail.isError}
        fallback={
          <>
            <PageHeader title={notFound() ? "Website not found" : "Website"} />
            <Alert variant="destructive">
              <AlertDescription>
                {notFound()
                  ? "This website does not exist or was deleted."
                  : describeError(detail.error)}
              </AlertDescription>
            </Alert>
            <Link to="/websites" class={`${buttonClasses("outline")} self-start`}>
              Back to websites
            </Link>
          </>
        }
      >
        <Show when={detail.data}>
          {(data) => (
            <>
              <PageHeader
                title={data().site.domain}
                description={`Created ${new Date(data().site.created_at).toLocaleDateString()} · WordPress on PHP ${data().site.php_version}`}
                actions={
                  <>
                    <Badge variant="outline" class={badgeTone(STATE_TONE[data().site.state])}>
                      {STATE_LABEL[data().site.state]}
                    </Badge>
                    <Show when={data().site.state === "ready"}>
                      <a
                        href={siteUrl(data().site.domain)}
                        target="_blank"
                        rel="noopener"
                        class={buttonClasses("outline")}
                      >
                        <ExternalLink aria-hidden /> Visit site
                      </a>
                    </Show>
                  </>
                }
              />

              <Show when={activeJob()?.id} keyed>
                {(jobId) => (
                  <Card class="p-5">
                    <JobProgress
                      jobId={jobId}
                      title={
                        detail.data?.job?.status === "failed"
                          ? "Build stopped"
                          : "Building your website"
                      }
                      onDismiss={() => setDismissedJob(jobId)}
                    />
                  </Card>
                )}
              </Show>

              <Tabs value={tab()} onValueChange={(value) => setTab(value)}>
                <TabsList>
                  <TabsTrigger value="overview">Overview</TabsTrigger>
                  <Show when={data().site.state === "ready" || data().site.state === "suspended"}>
                    <TabsTrigger value="files">Files</TabsTrigger>
                  </Show>
                  <TabsTrigger value="logs">Logs</TabsTrigger>
                  <TabsTrigger value="settings">Settings</TabsTrigger>
                </TabsList>

                <TabsContent value="overview">
                  <OverviewTab site={data().site} />
                </TabsContent>
                <Show when={data().site.state === "ready" || data().site.state === "suspended"}>
                  <TabsContent value="files">
                    <FileBrowser site={data().site} />
                  </TabsContent>
                </Show>
                <TabsContent value="logs">
                  <LogsTab site={data().site} />
                </TabsContent>
                <TabsContent value="settings">
                  <SettingsTab site={data().site} onDeleting={() => navigate("/websites")} />
                </TabsContent>
              </Tabs>
            </>
          )}
        </Show>
      </Show>
    </Show>
  );
}
