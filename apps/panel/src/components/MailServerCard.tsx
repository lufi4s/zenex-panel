import { createSignal, Show } from "solid-js";
import { describeError } from "@/api/client";
import { useApplyMailServer, useInstallMailServer, useSystemUpdate } from "@/api/queries";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

/** A full, lower-case host name such as mail.example.com. The server checks it again. */
const HOST_RE = /^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/;

/**
 * Sets up the mail server on this server in two steps: install the mail software (it runs the
 * panel's installer, like the panel update), then apply the mail configuration.
 * Administrators only.
 */
export function MailServerCard() {
  const status = useSystemUpdate(true);
  const install = useInstallMailServer();
  const apply = useApplyMailServer();
  const [host, setHost] = createSignal("");
  // Progress is shown only for an install started from this card, not for ordinary updates.
  const [started, setStarted] = createSignal(false);

  const hostValid = () => HOST_RE.test(host().trim());
  const running = () => status.data?.state === "running";

  const startInstall = () => {
    install.mutate(host().trim(), { onSuccess: () => setStarted(true) });
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Mail server</CardTitle>
        <CardDescription>
          Two steps set up mail on this server: install the mail software, then apply the mail
          settings of your domains. The server checks the settings first and keeps the old ones if
          the check fails.
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-6">
        <section class="space-y-3">
          <h3 class="text-sm font-semibold">Step 1: install the mail software</h3>
          <div class="space-y-1.5">
            <Label for="mail-host">Mail host name</Label>
            <Input
              id="mail-host"
              placeholder="mail.yourdomain.com"
              autocomplete="off"
              value={host()}
              onInput={(e) => setHost(e.currentTarget.value.trim().toLowerCase())}
            />
            <p class="text-xs text-muted-foreground">
              The name of this mail server. Its address (A record) must point to this server, and
              your hosting provider's reverse DNS (PTR) for this server's IP should be the same
              name.
            </p>
          </div>
          <div class="flex flex-wrap items-center gap-3">
            <Button
              disabled={!hostValid() || running() || install.isPending}
              onClick={startInstall}
            >
              {install.isPending ? "Starting…" : "Install mail server"}
            </Button>
            <Show when={running() && started()}>
              <span class="text-sm text-muted-foreground">
                Installing. This takes a few minutes and the panel may restart.
              </span>
            </Show>
            <Show when={started() && status.data?.state === "succeeded"}>
              <span class="text-sm text-success">Installed. Now do step 2.</span>
            </Show>
            <Show when={started() && status.data?.state === "failed"}>
              <span class="text-sm text-destructive">The install failed. See the log below.</span>
            </Show>
          </div>
          <Show when={install.isError}>
            <Alert variant="destructive">
              <AlertDescription>{describeError(install.error)}</AlertDescription>
            </Alert>
          </Show>
          <Show when={started() && status.data?.log}>
            <pre class="max-h-64 overflow-auto rounded-md border border-border bg-muted/40 p-3 text-xs">
              {status.data?.log}
            </pre>
          </Show>
        </section>

        <section class="space-y-3 border-t border-border pt-6">
          <h3 class="text-sm font-semibold">Step 2: apply the mail configuration</h3>
          <div class="flex flex-wrap items-center gap-3">
            <Button disabled={apply.isPending || running()} onClick={() => apply.mutate(undefined)}>
              {apply.isPending ? "Applying…" : "Apply mail configuration"}
            </Button>
            <Show when={apply.isSuccess}>
              <span class="text-sm text-success">Mail configuration applied.</span>
            </Show>
          </div>
          <Show when={apply.isError}>
            <Alert variant="destructive">
              <AlertDescription>{describeError(apply.error)}</AlertDescription>
            </Alert>
          </Show>
        </section>
      </CardContent>
    </Card>
  );
}
