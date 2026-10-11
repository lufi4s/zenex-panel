import { createSignal, Show } from "solid-js";
import { describeError } from "@/api/client";
import { useMailServer, useSetupMailServer } from "@/api/queries";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button, buttonClasses } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

/** A full, lower-case host name such as mail.example.com. The server checks it again. */
const HOST_RE = /^([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/;

/**
 * The mail software is installed together with the panel. This card sets it up: pick the mail
 * host name, and the server creates a certificate, opens the mail ports and loads the
 * configuration. It checks the configuration first and keeps the old one if the check fails.
 * Administrators only.
 */
export function MailServerCard() {
  const server = useMailServer();
  const setup = useSetupMailServer();
  const [typed, setTyped] = createSignal<string | null>(null);

  // The saved name until the administrator types another.
  const host = () => typed() ?? server.data?.host ?? "";
  const hostValid = () => HOST_RE.test(host().trim());
  const saved = () => server.data?.host ?? "";

  return (
    <Card>
      <CardHeader>
        <CardTitle>Mail server</CardTitle>
        <CardDescription>
          The mail software and the webmail come installed with the panel. Choose the mail host name
          to turn the mail server on; the webmail opens at the same name. Use it again after you
          change a domain's mail settings if mail does not pick them up.
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="space-y-1.5">
          <Label for="mail-host">Mail host name</Label>
          <Input
            id="mail-host"
            placeholder="mail.yourdomain.com"
            autocomplete="off"
            value={host()}
            onInput={(e) => setTyped(e.currentTarget.value.trim().toLowerCase())}
          />
          <p class="text-xs text-muted-foreground">
            The A record of this name must point to this server, and your hosting provider's reverse
            DNS (PTR) for this server's IP should be the same name.
          </p>
        </div>
        <div class="flex flex-wrap items-center gap-3">
          <Button
            disabled={!hostValid() || setup.isPending}
            onClick={() => setup.mutate(host().trim())}
          >
            {setup.isPending ? "Setting up…" : saved() ? "Save and apply" : "Set up mail server"}
          </Button>
          <Show when={setup.isSuccess}>
            <span class="text-sm text-success">Mail server is set up for {saved()}.</span>
          </Show>
          <Show when={!setup.isSuccess && saved()}>
            <span class="text-sm text-muted-foreground">Set up for {saved()}.</span>
          </Show>
          <Show when={server.data?.webmail_url}>
            {(url) => (
              <a
                href={url()}
                target="_blank"
                rel="noopener noreferrer"
                class={buttonClasses("outline", "sm")}
              >
                Open webmail
              </a>
            )}
          </Show>
        </div>
        <Show when={setup.isError}>
          <Alert variant="destructive">
            <AlertDescription>{describeError(setup.error)}</AlertDescription>
          </Alert>
        </Show>
      </CardContent>
    </Card>
  );
}
