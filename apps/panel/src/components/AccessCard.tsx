import { createSignal, Show } from "solid-js";
import { describeError } from "@/api/client";
import { useDisableSFTP, useOpenPhpMyAdmin, useSetSFTP, useSiteAccess } from "@/api/queries";
import type { Site } from "@/api/types";
import { Check, Copy, ExternalLink, Eye, EyeOff } from "@/components/icons";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

function errorOf(err: unknown, fallback: string): string {
  return err ? describeError(err) || fallback : fallback;
}

/** A value that can be copied. Secrets also start hidden and can be shown. */
function CopyValue(props: { value: string; label: string; secret?: boolean }) {
  const [shown, setShown] = createSignal(!props.secret);
  const [copied, setCopied] = createSignal(false);
  const copy = async () => {
    await navigator.clipboard.writeText(props.value);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };
  return (
    <span class="flex flex-wrap items-center gap-2">
      <span class="font-mono break-all">
        {shown() ? props.value : "•".repeat(Math.min(props.value.length, 16))}
      </span>
      <Show when={props.secret}>
        <Button
          variant="ghost"
          size="sm"
          aria-label={shown() ? `Hide ${props.label}` : `Show ${props.label}`}
          onClick={() => setShown((v) => !v)}
        >
          {shown() ? <EyeOff aria-hidden /> : <Eye aria-hidden />}
        </Button>
      </Show>
      <Button
        variant="ghost"
        size="sm"
        aria-label={`Copy ${props.label}`}
        onClick={() => void copy()}
      >
        {copied() ? <Check aria-hidden /> : <Copy aria-hidden />}
      </Button>
    </span>
  );
}

/** The database login for phpMyAdmin, and SFTP access to the website's own folder. */
export function AccessCard(props: { site: Site }) {
  const access = useSiteAccess(props.site.id);
  const pma = useOpenPhpMyAdmin();
  const setSFTP = useSetSFTP(props.site.id);
  const disableSFTP = useDisableSFTP(props.site.id);
  const [pmaError, setPmaError] = createSignal<string | null>(null);
  const live = () => props.site.state === "ready";
  const sftpBusy = () => setSFTP.isPending || disableSFTP.isPending;

  // The tab opens now, while the click still counts as a user action, then goes to phpMyAdmin.
  const openPhpMyAdmin = () => {
    setPmaError(null);
    const tab = window.open("about:blank", "_blank");
    if (!tab) {
      setPmaError("Allow pop-ups for this panel to open phpMyAdmin.");
      return;
    }
    tab.opener = null;
    pma.mutate(props.site.id, {
      onSuccess: (res) => {
        tab.location.href = res.url;
      },
      onError: (err) => {
        tab.close();
        setPmaError(errorOf(err, "Could not open phpMyAdmin."));
      },
    });
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Database and files</CardTitle>
        <CardDescription>
          Manage the website's database in phpMyAdmin, or move its files with SFTP.
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-6">
        <Show when={access.isError}>
          <Alert variant="destructive">
            <AlertDescription>
              {errorOf(access.error, "Could not load the access details.")}
            </AlertDescription>
          </Alert>
        </Show>

        <Show when={access.data}>
          {(a) => (
            <>
              <section class="space-y-3">
                <h3 class="text-sm font-semibold">Database (phpMyAdmin)</h3>
                <dl class="grid grid-cols-[auto_1fr] items-center gap-x-6 gap-y-3 text-sm">
                  <dt class="text-muted-foreground">Database</dt>
                  <dd class="font-mono">{a().database.name}</dd>
                  <dt class="text-muted-foreground">User</dt>
                  <dd class="font-mono">{a().database.user}</dd>
                  <dt class="text-muted-foreground">Password</dt>
                  <dd>
                    <CopyValue value={a().database.password} label="database password" secret />
                  </dd>
                </dl>
                <div class="space-y-2">
                  <Button
                    disabled={!live() || props.site.maintenance || pma.isPending}
                    onClick={openPhpMyAdmin}
                  >
                    <ExternalLink aria-hidden /> {pma.isPending ? "Opening…" : "Open phpMyAdmin"}
                  </Button>
                  <Show when={props.site.maintenance}>
                    <p class="text-xs text-muted-foreground">
                      Turn off maintenance mode on the Overview tab to open phpMyAdmin.
                    </p>
                  </Show>
                  <Show when={pmaError()}>
                    {(message) => (
                      <Alert variant="destructive">
                        <AlertDescription>{message()}</AlertDescription>
                      </Alert>
                    )}
                  </Show>
                </div>
                <p class="text-xs text-muted-foreground">
                  phpMyAdmin only opens this website's database with the login above.
                </p>
              </section>

              <section class="space-y-3 border-t border-border pt-6">
                <h3 class="text-sm font-semibold">SFTP (file transfer)</h3>
                <Show
                  when={a().sftp.enabled}
                  fallback={
                    <p class="text-sm text-muted-foreground">
                      SFTP is off. Turn it on to connect with an FTP client such as FileZilla. It
                      can only reach this website's folder.
                    </p>
                  }
                >
                  <dl class="grid grid-cols-[auto_1fr] items-center gap-x-6 gap-y-3 text-sm">
                    <dt class="text-muted-foreground">Host</dt>
                    <dd class="font-mono">{a().sftp.host}</dd>
                    <dt class="text-muted-foreground">Port</dt>
                    <dd class="font-mono">{a().sftp.port}</dd>
                    <dt class="text-muted-foreground">User</dt>
                    <dd class="font-mono">{a().sftp.user}</dd>
                    <dt class="text-muted-foreground">Password</dt>
                    <dd>
                      <CopyValue value={a().sftp.password} label="SFTP password" secret />
                    </dd>
                  </dl>
                </Show>
                <div class="flex flex-wrap gap-2">
                  <Show
                    when={a().sftp.enabled}
                    fallback={
                      <Button
                        variant="outline"
                        disabled={!live() || sftpBusy()}
                        onClick={() => setSFTP.mutate(false)}
                      >
                        Turn on SFTP
                      </Button>
                    }
                  >
                    <Button
                      variant="outline"
                      disabled={!live() || sftpBusy()}
                      onClick={() => setSFTP.mutate(true)}
                    >
                      New password
                    </Button>
                    <Button
                      variant="outline"
                      disabled={!live() || sftpBusy()}
                      onClick={() => disableSFTP.mutate(undefined)}
                    >
                      Turn off SFTP
                    </Button>
                  </Show>
                </div>
                <Show when={!live()}>
                  <p class="text-xs text-muted-foreground">
                    SFTP can be changed once the website is live.
                  </p>
                </Show>
                <Show when={setSFTP.error || disableSFTP.error}>
                  <Alert variant="destructive">
                    <AlertDescription>
                      {errorOf(
                        setSFTP.error || disableSFTP.error,
                        "The SFTP change did not work. Try again.",
                      )}
                    </AlertDescription>
                  </Alert>
                </Show>
              </section>
            </>
          )}
        </Show>
      </CardContent>
    </Card>
  );
}
