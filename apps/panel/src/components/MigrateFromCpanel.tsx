import { createSignal, For, Show } from "solid-js";
import { describeError } from "@/api/client";
import { useCpanelMigrate, useCpanelScan, useJob } from "@/api/queries";
import type { CpanelConn, CpanelInstall, Site } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { JobProgress } from "@/components/JobProgress";
import { Server } from "@/components/icons";
import { formatBytes } from "@/lib/format";
import { Link, useNavigate } from "@/lib/router";

/** The migration that is running: the website it fills and the job to follow. */
interface Run {
  site: Site;
  jobId: string;
  path: string;
  domain: string;
  sourceDomain: string;
}

/** The two last labels of a host, as a guess for the domain to add (www.example.com -> example.com). */
function domainGuess(host: string): string {
  return host.split(".").slice(-2).join(".");
}

function InstallRow(props: {
  install: CpanelInstall;
  disabled: boolean;
  onMigrate: (install: CpanelInstall, domain: string, replaceSiteId?: string) => void;
}) {
  const [typed, setTyped] = createSignal(props.install.domain);
  const domain = () => typed().trim().toLowerCase();
  const known = () => props.install.domain !== "";
  const exists = () => props.install.existing_site_id !== "";
  const ready = () => known() && props.install.matched_domain !== "";

  const replace = () => {
    const ok = window.confirm(
      `This replaces the files and the database of ${domain()} on this panel with the copy from cPanel. Continue?`,
    );
    if (ok) props.onMigrate(props.install, domain(), props.install.existing_site_id);
  };

  return (
    <li class="space-y-3 rounded-lg border border-border p-4">
      <div class="flex flex-wrap items-start justify-between gap-3">
        <div class="min-w-0">
          <p class="break-all font-medium">{props.install.domain || "Address not found"}</p>
          <p class="break-all font-mono text-xs text-muted-foreground">{props.install.path}</p>
          <p class="text-xs text-muted-foreground">
            {props.install.size_kb > 0 ? formatBytes(props.install.size_kb * 1024) : "Size unknown"}
          </p>
        </div>
        <Show
          when={exists()}
          fallback={
            <Button
              size="sm"
              disabled={props.disabled || (known() ? !ready() : domain() === "")}
              onClick={() => props.onMigrate(props.install, domain())}
            >
              Migrate
            </Button>
          }
        >
          <Button size="sm" variant="destructive" disabled={props.disabled} onClick={replace}>
            Replace its content
          </Button>
        </Show>
      </div>

      <Show when={!known()}>
        <div class="space-y-1.5">
          <Label for={`migrate-domain-${props.install.path}`}>Address of this website</Label>
          <Input
            id={`migrate-domain-${props.install.path}`}
            placeholder="www.example.com"
            autocomplete="off"
            value={typed()}
            onInput={(e) => setTyped(e.currentTarget.value)}
          />
          <p class="text-xs text-muted-foreground">
            The address could not be read from the database. It must belong to a domain you added
            under Domains.
          </p>
        </div>
      </Show>
      <Show when={exists()}>
        <p class="text-xs text-warning">
          A website with this address already exists on this panel.
        </p>
      </Show>
      <Show when={known() && !exists() && !ready()}>
        <p class="text-xs text-warning">
          Add <span class="font-mono">{domainGuess(props.install.domain)}</span> under{" "}
          <Link to="/domains" class="underline underline-offset-4">
            Domains
          </Link>{" "}
          first. Its DNS can stay on cPanel for now.
        </p>
      </Show>
    </li>
  );
}

/** Progress of one migration, the DNS instructions, and a retry when it stops. */
function RunningView(props: {
  run: Run;
  serverIp: string;
  starting: boolean;
  onRetry: () => void;
  onDone: () => void;
}) {
  const navigate = useNavigate();
  const job = useJob(props.run.jobId);
  const status = () => job.data?.job.status;
  const failed = () => status() === "failed" || status() === "dead";

  return (
    <div class="space-y-4">
      <JobProgress
        jobId={props.run.jobId}
        title="Moving your website"
        retryable={false}
        onDismiss={props.onDone}
      />
      <Show when={failed()}>
        <div class="flex flex-wrap items-center justify-end gap-2">
          <p class="mr-auto text-xs text-muted-foreground">
            The cPanel account was not changed. Retrying starts the copy again.
          </p>
          <Button variant="outline" size="sm" disabled={props.starting} onClick={props.onRetry}>
            {props.starting ? "Starting…" : "Try again"}
          </Button>
        </div>
      </Show>
      <Show when={status() === "succeeded"}>
        <Alert>
          <AlertDescription class="space-y-2">
            <p>
              <strong>{props.run.domain}</strong> is ready on this panel. Your visitors still see
              the cPanel website until you switch the DNS.
            </p>
            <p>
              To go live, set the DNS A record of <span class="font-mono">{props.run.domain}</span>{" "}
              to <span class="font-mono">{props.serverIp || "this server's IP address"}</span>. The
              HTTPS certificate is issued automatically once the DNS points here.
            </p>
          </AlertDescription>
        </Alert>
        <div class="flex justify-end">
          <Button onClick={() => navigate(`/websites/${props.run.site.id}`)}>Open website</Button>
        </div>
      </Show>
    </div>
  );
}

/**
 * Brings a WordPress website over from a cPanel account. It signs in over SSH and only reads:
 * nothing on the cPanel account is changed, and the DNS is not touched. The password is kept in
 * this window only and is forgotten when it closes.
 */
export function MigrateFromCpanel() {
  const scan = useCpanelScan();
  const migrate = useCpanelMigrate();
  const [open, setOpen] = createSignal(false);
  const [host, setHost] = createSignal("");
  const [port, setPort] = createSignal("22");
  const [username, setUsername] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [scanned, setScanned] = createSignal(false);
  const [installs, setInstalls] = createSignal<CpanelInstall[]>([]);
  const [serverIp, setServerIp] = createSignal("");
  const [run, setRun] = createSignal<Run | null>(null);
  const [error, setError] = createSignal<string | null>(null);

  const conn = (): CpanelConn => ({
    host: host().trim(),
    port: Number(port()) || 22,
    username: username().trim(),
    password: password(),
  });
  const canScan = () =>
    host().trim() !== "" && username().trim() !== "" && password() !== "" && !scan.isPending;

  const reset = () => {
    setPassword("");
    setScanned(false);
    setInstalls([]);
    setRun(null);
    setError(null);
    scan.reset();
    migrate.reset();
  };

  const find = (event: SubmitEvent) => {
    event.preventDefault();
    if (!canScan()) return;
    setError(null);
    scan.mutate(conn(), {
      onSuccess: (res) => {
        setInstalls(res.installs);
        setServerIp(res.server_ip);
        setScanned(true);
      },
      onError: (err) => setError(describeError(err)),
    });
  };

  const start = (install: CpanelInstall, domain: string, siteId?: string) => {
    setError(null);
    migrate.mutate(
      {
        ...conn(),
        path: install.path,
        domain,
        source_domain: install.domain || domain,
        site_id: siteId,
      },
      {
        onSuccess: (res) => {
          setServerIp(res.server_ip || serverIp());
          setRun({
            site: res.site,
            jobId: res.job_id,
            path: install.path,
            domain: res.site.domain,
            sourceDomain: install.domain || domain,
          });
        },
        onError: (err) => setError(describeError(err)),
      },
    );
  };

  const retry = () => {
    const current = run();
    if (!current) return;
    migrate.mutate(
      {
        ...conn(),
        path: current.path,
        domain: current.domain,
        source_domain: current.sourceDomain,
        site_id: current.site.id,
      },
      {
        onSuccess: (res) => setRun({ ...current, jobId: res.job_id }),
        onError: (err) => setError(describeError(err)),
      },
    );
  };

  return (
    <>
      <Button variant="outline" onClick={() => setOpen(true)}>
        <Server aria-hidden /> Migrate from cPanel
      </Button>
      <Dialog
        open={open()}
        onOpenChange={(next) => {
          setOpen(next);
          if (!next) reset();
        }}
      >
        <DialogContent class="max-h-[90vh] overflow-y-auto sm:max-w-2xl">
          <DialogHeader>
            <DialogTitle>Migrate from cPanel</DialogTitle>
            <DialogDescription>
              Sign in to the cPanel account with SSH. The panel finds your WordPress websites and
              copies the files and the database. Nothing on cPanel is changed, and the DNS stays
              where it is until you switch it.
            </DialogDescription>
          </DialogHeader>

          <Show when={error()}>
            {(message) => (
              <Alert variant="destructive">
                <AlertDescription>{message()}</AlertDescription>
              </Alert>
            )}
          </Show>

          <Show
            when={run()}
            keyed
            fallback={
              <Show
                when={scanned()}
                fallback={
                  <form class="grid gap-4 sm:grid-cols-[1fr_7rem]" onSubmit={find} noValidate>
                    <div class="space-y-1.5">
                      <Label for="cpanel-host">cPanel server</Label>
                      <Input
                        id="cpanel-host"
                        placeholder="server.example.com"
                        autocomplete="off"
                        value={host()}
                        onInput={(e) => setHost(e.currentTarget.value)}
                      />
                    </div>
                    <div class="space-y-1.5">
                      <Label for="cpanel-port">SSH port</Label>
                      <Input
                        id="cpanel-port"
                        type="number"
                        min={1}
                        max={65535}
                        value={port()}
                        onInput={(e) => setPort(e.currentTarget.value)}
                      />
                    </div>
                    <div class="space-y-1.5">
                      <Label for="cpanel-user">cPanel username</Label>
                      <Input
                        id="cpanel-user"
                        autocomplete="off"
                        value={username()}
                        onInput={(e) => setUsername(e.currentTarget.value)}
                      />
                    </div>
                    <div class="space-y-1.5 sm:col-span-2">
                      <Label for="cpanel-password">cPanel password</Label>
                      <Input
                        id="cpanel-password"
                        type="password"
                        autocomplete="new-password"
                        value={password()}
                        onInput={(e) => setPassword(e.currentTarget.value)}
                      />
                      <p class="text-xs text-muted-foreground">
                        SSH access must be turned on for the account. The password is used for this
                        migration only and is not saved.
                      </p>
                    </div>
                    <div class="flex justify-end sm:col-span-2">
                      <Button type="submit" disabled={!canScan()}>
                        {scan.isPending ? "Looking for websites…" : "Find websites"}
                      </Button>
                    </div>
                  </form>
                }
              >
                <div class="space-y-3">
                  <Show
                    when={installs().length > 0}
                    fallback={
                      <p class="text-sm text-muted-foreground">
                        No WordPress websites were found in this account.
                      </p>
                    }
                  >
                    <p class="text-sm text-muted-foreground">
                      {installs().length === 1
                        ? "1 WordPress website found."
                        : `${installs().length} WordPress websites found.`}{" "}
                      Choose the one to move.
                    </p>
                    <ul class="space-y-3">
                      <For each={installs()}>
                        {(install) => (
                          <InstallRow
                            install={install}
                            disabled={migrate.isPending}
                            onMigrate={start}
                          />
                        )}
                      </For>
                    </ul>
                  </Show>
                  <div class="flex justify-between">
                    <Button variant="ghost" size="sm" onClick={() => setScanned(false)}>
                      Back
                    </Button>
                  </div>
                </div>
              </Show>
            }
          >
            {(current) => (
              <RunningView
                run={current}
                serverIp={serverIp()}
                starting={migrate.isPending}
                onRetry={retry}
                onDone={() => setOpen(false)}
              />
            )}
          </Show>
        </DialogContent>
      </Dialog>
    </>
  );
}
