import { createEffect, createSignal, For, onMount, Show } from "solid-js";
import { useQueryClient } from "@/api/query";
import { Check, Copy, Eye, EyeOff, ExternalLink, RotateCcw } from "@/components/icons";
import { Link, useNavigate, useParams } from "@/lib/router";
import {
  useBackupNow,
  useChangePHP,
  useCredentials,
  useDeleteSite,
  usePHPVersions,
  useSetAutoUpdate,
  useSetMaintenance,
  useSite,
  useSiteAction,
  useSiteBackups,
  useSiteHealth,
  useRestoreBackup,
  useSiteLogs,
  useWPLoginLink,
} from "@/api/queries";
import { ApiError, describeError } from "@/api/client";
import type { Site, SiteBackup } from "@/api/types";
import { Select } from "@/components/ui/select";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { BackupProgress } from "@/components/BackupProgress";
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
import { cn } from "@/lib/utils";
import { formatBytes, siteUrl } from "@/lib/format";
import { STATE_LABEL, STATE_TONE } from "@/lib/site-status";

function errorOf(err: unknown, fallback: string): string {
  return err ? describeError(err) || fallback : fallback;
}

// ---------------------------------------------------------------------------
// Overview tab
// ---------------------------------------------------------------------------

/** The five newest PHP releases offered for a website. */
const LATEST_PHP = ["8.5", "8.4", "8.3", "8.2", "8.1"];

/**
 * WordPress admin access: the admin link and the stored login. The panel and the site are
 * different sites, so the browser will not keep a login that is posted from the panel.
 */
function WordPressAccess(props: { site: Site }) {
  const creds = useCredentials();
  const loginLink = useWPLoginLink();
  const [showPassword, setShowPassword] = createSignal(false);
  const [copied, setCopied] = createSignal(false);
  const [adminError, setAdminError] = createSignal<string | null>(null);

  // The tab opens now, while the click still counts as a user action (pop-up blockers allow it);
  // it is sent to the sign-in link once the panel has prepared it.
  const openAdmin = () => {
    setAdminError(null);
    const tab = window.open("about:blank", "_blank");
    if (!tab) {
      setAdminError("Allow pop-ups for this panel to open the admin dashboard.");
      return;
    }
    tab.opener = null;
    loginLink.mutate(props.site.id, {
      onSuccess: (res) => {
        tab.location.href = res.url;
      },
      onError: (err) => {
        tab.close();
        setAdminError(errorOf(err, "Could not open the admin dashboard."));
      },
    });
  };

  onMount(() => {
    if (props.site.state === "ready") creds.mutate(props.site.id);
  });

  const copyPassword = async (password: string) => {
    await navigator.clipboard.writeText(password);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>WordPress</CardTitle>
        <CardDescription>
          Open the admin dashboard and you are signed in automatically. The username and password
          below sign you in later.
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="space-y-2">
          <Button
            disabled={props.site.state !== "ready" || loginLink.isPending}
            onClick={openAdmin}
          >
            <ExternalLink aria-hidden /> {loginLink.isPending ? "Opening…" : "Open admin"}
          </Button>
          <Show when={adminError()}>
            {(message) => (
              <Alert variant="destructive">
                <AlertDescription>{message()}</AlertDescription>
              </Alert>
            )}
          </Show>
        </div>
        <Show when={creds.isError}>
          <Alert variant="destructive">
            <AlertDescription>{errorOf(creds.error, "Could not load the login.")}</AlertDescription>
          </Alert>
        </Show>
        <Show when={creds.data}>
          {(c) => (
            <dl class="grid grid-cols-[auto_1fr] items-center gap-x-6 gap-y-3 text-sm">
              <dt class="text-muted-foreground">Username</dt>
              <dd class="font-mono">{c().username}</dd>
              <dt class="text-muted-foreground">Password</dt>
              <dd class="flex flex-wrap items-center gap-2">
                <span class="font-mono break-all">
                  {showPassword() ? c().password : "•".repeat(Math.min(c().password.length, 16))}
                </span>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label={showPassword() ? "Hide password" : "Show password"}
                  onClick={() => setShowPassword((v) => !v)}
                >
                  {showPassword() ? <EyeOff aria-hidden /> : <Eye aria-hidden />}
                </Button>
                <Button
                  variant="ghost"
                  size="sm"
                  aria-label="Copy password"
                  onClick={() => void copyPassword(c().password)}
                >
                  {copied() ? <Check aria-hidden /> : <Copy aria-hidden />}
                </Button>
              </dd>
            </dl>
          )}
        </Show>
      </CardContent>
    </Card>
  );
}

/** Turns maintenance mode on or off. Visitors see a maintenance page while it is on. */
function MaintenanceCard(props: { site: Site }) {
  const toggle = useSetMaintenance(props.site.id);
  const on = () => props.site.maintenance;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Maintenance mode</CardTitle>
        <CardDescription>Pause public access while you make changes.</CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="flex items-center justify-between gap-3">
          <span class="text-sm font-medium">{on() ? "On" : "Off"}</span>
          <button
            type="button"
            role="switch"
            aria-checked={on()}
            aria-label="Maintenance mode"
            disabled={toggle.isPending}
            onClick={() => toggle.mutate(!on())}
            class={cn(
              "relative inline-flex h-6 w-11 shrink-0 items-center rounded-full transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:opacity-50",
              on() ? "bg-primary" : "bg-muted",
            )}
          >
            <span
              class={cn(
                "inline-block size-5 rounded-full bg-background shadow transition-transform",
                on() ? "translate-x-5" : "translate-x-0.5",
              )}
            />
          </button>
        </div>
        <Show when={on()}>
          <Alert>
            <AlertDescription>
              Maintenance mode is on. Visitors see a maintenance page instead of your website.
            </AlertDescription>
          </Alert>
        </Show>
        <Show when={toggle.isError}>
          <Alert variant="destructive">
            <AlertDescription>
              {errorOf(toggle.error, "Could not change maintenance mode.")}
            </AlertDescription>
          </Alert>
        </Show>
      </CardContent>
    </Card>
  );
}

/** Lets WordPress core update this website automatically once a day. */
function AutoUpdateCard(props: { site: Site }) {
  const toggle = useSetAutoUpdate(props.site.id);

  return (
    <Card>
      <CardHeader>
        <CardTitle>WordPress updates</CardTitle>
        <CardDescription>Keep WordPress up to date without manual work.</CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <label class="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            class="size-4 accent-primary"
            checked={props.site.auto_update}
            disabled={toggle.isPending}
            onChange={(e) => {
              const input = e.currentTarget;
              toggle.mutate(input.checked, {
                // Put the box back to the saved value if the change was refused.
                onError: () => {
                  input.checked = props.site.auto_update;
                },
              });
            }}
          />
          Update automatically every day
        </label>
        <Show when={toggle.isError}>
          <Alert variant="destructive">
            <AlertDescription>
              {errorOf(toggle.error, "Could not change automatic updates.")}
            </AlertDescription>
          </Alert>
        </Show>
      </CardContent>
    </Card>
  );
}

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
  // The five newest PHP releases, plus the site's current version if it is older.
  const versions = () =>
    LATEST_PHP.includes(props.site.php_version)
      ? LATEST_PHP
      : [props.site.php_version, ...LATEST_PHP];
  const installed = () => phpVersions.data ?? [];
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
        <MaintenanceCard site={props.site} />
        <AutoUpdateCard site={props.site} />
      </Show>

      <Show when={ready()}>
        <div class="lg:col-span-3">
          <WordPressAccess site={props.site} />
        </div>
      </Show>

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
                <Select id="php-version" onChange={(e) => setPhpChoice(e.currentTarget.value)}>
                  <For each={versions()}>
                    {(v) => {
                      const available = () =>
                        installed().includes(v) || v === props.site.php_version;
                      return (
                        <option value={v} selected={v === phpChoice()} disabled={!available()}>
                          PHP {v}
                          {available() ? "" : " (not installed on this server)"}
                        </option>
                      );
                    }}
                  </For>
                </Select>
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
        if (next) del.reset();
        else setTyped("");
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

/** Lists the website's backups, newest first, and starts a backup on demand. */
function BackupsCard(props: { site: Site }) {
  const qc = useQueryClient();
  const backups = useSiteBackups(props.site.id);
  const backupNow = useBackupNow(props.site.id);
  const restore = useRestoreBackup(props.site.id);
  // The backup the customer is about to restore, and the restore job once it has started.
  const [restoring, setRestoring] = createSignal<SiteBackup | null>(null);
  const [restoreJob, setRestoreJob] = createSignal<string | null>(null);
  const rows = () =>
    [...(backups.data ?? [])].sort(
      (a, b) => new Date(b.created_at).getTime() - new Date(a.created_at).getTime(),
    );
  const restoringAt = () => {
    const backup = restoring();
    return backup ? new Date(backup.created_at).toLocaleString() : "";
  };

  const confirmRestore = () => {
    const backup = restoring();
    if (!backup) return;
    restore.mutate(backup.id, {
      onSuccess: (res) => {
        setRestoring(null);
        setRestoreJob(res.job_id);
      },
    });
  };

  return (
    <Card class="lg:col-span-2">
      <CardHeader>
        <CardTitle>Backups</CardTitle>
        <CardDescription>Copies of this website&apos;s files and database.</CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div>
          <Button
            variant="outline"
            disabled={backupNow.isPending || props.site.state !== "ready"}
            onClick={() => backupNow.mutate()}
          >
            {backupNow.isPending ? "Starting…" : "Back up now"}
          </Button>
        </div>
        <BackupProgress siteId={props.site.id} />
        <Show when={backupNow.isError}>
          <Alert variant="destructive">
            <AlertDescription>
              {errorOf(backupNow.error, "Could not start a backup.")}
            </AlertDescription>
          </Alert>
        </Show>
        <Show when={restoreJob()} keyed>
          {(jobId) => (
            <JobProgress
              jobId={jobId}
              title="Restoring backup"
              retryable={false}
              onDismiss={() => {
                setRestoreJob(null);
                void qc.invalidateQueries({ queryKey: ["backups", props.site.id] });
              }}
            />
          )}
        </Show>
        <Show when={restore.isError}>
          <Alert variant="destructive">
            <AlertDescription>
              {errorOf(restore.error, "Could not start the restore.")}
            </AlertDescription>
          </Alert>
        </Show>
        <Show when={backups.isError}>
          <Alert variant="destructive">
            <AlertDescription>{errorOf(backups.error, "Could not load backups.")}</AlertDescription>
          </Alert>
        </Show>
        <Show
          when={!backups.isPending}
          fallback={<p class="text-sm text-muted-foreground">Loading backups…</p>}
        >
          <Show
            when={rows().length > 0}
            fallback={
              <p class="text-sm text-muted-foreground">
                No backups yet. Back up now to make the first copy.
              </p>
            }
          >
            <div class="overflow-x-auto">
              <table class="w-full text-sm">
                <thead>
                  <tr class="border-b border-border text-left text-muted-foreground">
                    <th scope="col" class="py-2 pr-4 font-medium">
                      Date
                    </th>
                    <th scope="col" class="py-2 pr-4 text-right font-medium">
                      Size
                    </th>
                    <th scope="col" class="py-2 text-right font-medium">
                      <span class="sr-only">Actions</span>
                    </th>
                  </tr>
                </thead>
                <tbody>
                  <For each={rows()}>
                    {(backup) => (
                      <tr class="border-b border-border last:border-0">
                        <td class="py-2 pr-4 tabular-nums">
                          {new Date(backup.created_at).toLocaleString()}
                        </td>
                        <td class="py-2 pr-4 text-right tabular-nums">
                          {formatBytes(backup.size_bytes)}
                        </td>
                        <td class="py-2 text-right">
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={props.site.state !== "ready" || restore.isPending}
                            onClick={() => {
                              restore.reset();
                              setRestoring(backup);
                            }}
                          >
                            <RotateCcw aria-hidden="true" />
                            Restore
                          </Button>
                        </td>
                      </tr>
                    )}
                  </For>
                </tbody>
              </table>
            </div>
          </Show>
        </Show>
      </CardContent>
      <Dialog
        open={restoring() !== null}
        onOpenChange={(next) => {
          if (!next) setRestoring(null);
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Restore this backup?</DialogTitle>
            <DialogDescription>
              The files and database of {props.site.domain} are replaced with the backup from{" "}
              {restoringAt()}. A copy of the current website is saved first, so you can restore it
              again from the list.
            </DialogDescription>
          </DialogHeader>
          <div class="flex justify-end gap-2">
            <Button variant="outline" onClick={() => setRestoring(null)}>
              Cancel
            </Button>
            <Button variant="destructive" disabled={restore.isPending} onClick={confirmRestore}>
              {restore.isPending ? "Starting…" : "Restore backup"}
            </Button>
          </div>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

function SettingsTab(props: { site: Site; onDeleting: () => void }) {
  return (
    <div class="grid gap-4 lg:grid-cols-2">
      <BackupsCard site={props.site} />
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
  const filesAvailable = () => {
    const state = detail.data?.site.state;
    return state === "ready" || state === "suspended";
  };
  // The Files tab disappears when the website goes offline for a build; do not stay on it.
  createEffect(() => {
    if (detail.data && !filesAvailable() && tab() === "files") setTab("overview");
  });

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
