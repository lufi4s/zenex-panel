import { createSignal, Show } from "solid-js";
import { useSaveAlerts, useSendTestEmail, useSettingsAlerts } from "@/api/queries";
import { describeError } from "@/api/client";
import type { AlertSettings, AlertSettingsInput } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CardSkeleton } from "@/components/CardSkeleton";

const EMPTY: AlertSettingsInput = {
  email: { enabled: false, host: "", port: 587, username: "", from: "", to: "", password: "" },
  telegram: { enabled: false, chat_id: "", bot_token: "" },
  thresholds: { cpu: 85, memory: 90, disk: 90 },
};

/** The form starts from what the server stores. Secrets are never returned, so they start blank. */
function toForm(s: AlertSettings): AlertSettingsInput {
  return {
    email: {
      enabled: s.email.enabled,
      host: s.email.host,
      port: s.email.port,
      username: s.email.username,
      from: s.email.from,
      to: s.email.to,
      password: "",
    },
    telegram: { enabled: s.telegram.enabled, chat_id: s.telegram.chat_id, bot_token: "" },
    thresholds: { ...s.thresholds },
  };
}

/** Parses a number field. An empty field is NaN, which fails validation. */
const parseField = (text: string) => (text.trim() === "" ? Number.NaN : Number(text));
const inRange = (n: number, low: number, high: number) =>
  Number.isInteger(n) && n >= low && n <= high;

const FIELD_CLASS = "space-y-2";

/** Administrators set where alerts are sent and the usage levels that trigger them. */
export function AlertSettingsCard() {
  const settings = useSettingsAlerts();
  const save = useSaveAlerts();
  const testEmail = useSendTestEmail();
  const [draft, setDraft] = createSignal<AlertSettingsInput | null>(null);

  const baseline = (): AlertSettingsInput => (settings.data ? toForm(settings.data) : EMPTY);
  const value = (): AlertSettingsInput => draft() ?? baseline();
  // Typing a new password or token makes the form dirty, even though the stored one is unseen.
  const dirty = () =>
    settings.data !== undefined && JSON.stringify(value()) !== JSON.stringify(baseline());

  const portValid = () => inRange(value().email.port, 1, 65535);
  const thresholdValid = (n: number) => inRange(n, 1, 100);
  const thresholdsValid = () =>
    thresholdValid(value().thresholds.cpu) &&
    thresholdValid(value().thresholds.memory) &&
    thresholdValid(value().thresholds.disk);
  const canSave = () => dirty() && portValid() && thresholdsValid() && !save.isPending;

  const setEmail = (patch: Partial<AlertSettingsInput["email"]>) =>
    setDraft({ ...value(), email: { ...value().email, ...patch } });
  const useGmail = () => setEmail({ host: "smtp.gmail.com", port: 587 });
  const setTelegram = (patch: Partial<AlertSettingsInput["telegram"]>) =>
    setDraft({ ...value(), telegram: { ...value().telegram, ...patch } });
  const setThresholds = (patch: Partial<AlertSettingsInput["thresholds"]>) =>
    setDraft({ ...value(), thresholds: { ...value().thresholds, ...patch } });

  const submit = (event: SubmitEvent) => {
    event.preventDefault();
    if (!canSave()) return;
    const v = value();
    save.mutate(
      {
        email: {
          ...v.email,
          host: v.email.host.trim(),
          username: v.email.username.trim(),
          from: v.email.from.trim(),
          to: v.email.to.trim(),
        },
        telegram: { ...v.telegram, chat_id: v.telegram.chat_id.trim() },
        thresholds: v.thresholds,
      },
      { onSuccess: () => setDraft(null) },
    );
  };

  const percentField = (id: string, label: string, key: keyof AlertSettingsInput["thresholds"]) => (
    <div class={FIELD_CLASS}>
      <Label for={id}>{label}</Label>
      <div class="flex items-center gap-2">
        <Input
          id={id}
          type="number"
          min={1}
          max={100}
          step={1}
          class="w-24"
          value={Number.isNaN(value().thresholds[key]) ? "" : value().thresholds[key]}
          onInput={(e) => setThresholds({ [key]: parseField(e.currentTarget.value) })}
          aria-invalid={!thresholdValid(value().thresholds[key]) || undefined}
        />
        <span class="text-sm text-muted-foreground">%</span>
      </div>
    </div>
  );

  return (
    <Show when={!settings.isPending} fallback={<CardSkeleton height="h-[32rem]" />}>
      <Card>
        <CardHeader>
          <CardTitle>Alerts</CardTitle>
          <CardDescription>
            Where the panel sends alerts when a server or website needs attention.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form class="space-y-7" onSubmit={submit} noValidate>
            <Show when={settings.isError}>
              <Alert variant="destructive">
                <AlertDescription>{describeError(settings.error)}</AlertDescription>
              </Alert>
            </Show>

            <fieldset class="space-y-4">
              <legend class="mb-3 text-sm font-semibold">Email</legend>
              <label class="flex items-center gap-2 text-sm">
                <input
                  id="alert-email-enabled"
                  type="checkbox"
                  class="size-4 accent-primary"
                  checked={value().email.enabled}
                  onChange={(e) => setEmail({ enabled: e.currentTarget.checked })}
                />
                Send alerts by email
              </label>
              <p class="text-xs text-muted-foreground">
                Use Gmail: host smtp.gmail.com, port 587, your Gmail address as the username and as
                'From', and a Google App Password (not your normal password). Mail sent from the
                same address lands in the inbox more reliably.
              </p>
              <div>
                <Button type="button" variant="outline" size="sm" onClick={useGmail}>
                  Use Gmail settings
                </Button>
              </div>
              <div class="grid gap-4 sm:grid-cols-2">
                <div class={FIELD_CLASS}>
                  <Label for="alert-smtp-host">SMTP host</Label>
                  <Input
                    id="alert-smtp-host"
                    value={value().email.host}
                    onInput={(e) => setEmail({ host: e.currentTarget.value })}
                  />
                </div>
                <div class={FIELD_CLASS}>
                  <Label for="alert-smtp-port">SMTP port</Label>
                  <Input
                    id="alert-smtp-port"
                    type="number"
                    min={1}
                    max={65535}
                    step={1}
                    class="w-28"
                    value={Number.isNaN(value().email.port) ? "" : value().email.port}
                    onInput={(e) => setEmail({ port: parseField(e.currentTarget.value) })}
                    aria-invalid={!portValid() || undefined}
                  />
                </div>
                <div class={FIELD_CLASS}>
                  <Label for="alert-smtp-username">Username</Label>
                  <Input
                    id="alert-smtp-username"
                    autocomplete="off"
                    value={value().email.username}
                    onInput={(e) => setEmail({ username: e.currentTarget.value })}
                  />
                </div>
                <div class={FIELD_CLASS}>
                  <Label for="alert-smtp-password">Password</Label>
                  <Input
                    id="alert-smtp-password"
                    type="password"
                    autocomplete="new-password"
                    value={value().email.password}
                    placeholder={
                      settings.data?.email.password_set
                        ? "Leave blank to keep the saved password"
                        : undefined
                    }
                    onInput={(e) => setEmail({ password: e.currentTarget.value })}
                  />
                </div>
                <div class={FIELD_CLASS}>
                  <Label for="alert-email-from">From address</Label>
                  <Input
                    id="alert-email-from"
                    type="email"
                    value={value().email.from}
                    onInput={(e) => setEmail({ from: e.currentTarget.value })}
                  />
                </div>
                <div class={FIELD_CLASS}>
                  <Label for="alert-email-to">Send to</Label>
                  <Input
                    id="alert-email-to"
                    type="email"
                    value={value().email.to}
                    onInput={(e) => setEmail({ to: e.currentTarget.value })}
                  />
                </div>
              </div>
            </fieldset>

            <fieldset class="space-y-4">
              <legend class="mb-3 text-sm font-semibold">Telegram</legend>
              <label class="flex items-center gap-2 text-sm">
                <input
                  id="alert-telegram-enabled"
                  type="checkbox"
                  class="size-4 accent-primary"
                  checked={value().telegram.enabled}
                  onChange={(e) => setTelegram({ enabled: e.currentTarget.checked })}
                />
                Send alerts to Telegram
              </label>
              <div class="grid gap-4 sm:grid-cols-2">
                <div class={FIELD_CLASS}>
                  <Label for="alert-telegram-chat">Chat ID</Label>
                  <Input
                    id="alert-telegram-chat"
                    autocomplete="off"
                    value={value().telegram.chat_id}
                    onInput={(e) => setTelegram({ chat_id: e.currentTarget.value })}
                  />
                </div>
                <div class={FIELD_CLASS}>
                  <Label for="alert-telegram-token">Bot token</Label>
                  <Input
                    id="alert-telegram-token"
                    type="password"
                    autocomplete="new-password"
                    value={value().telegram.bot_token}
                    placeholder={
                      settings.data?.telegram.token_set
                        ? "Leave blank to keep the saved token"
                        : undefined
                    }
                    onInput={(e) => setTelegram({ bot_token: e.currentTarget.value })}
                  />
                </div>
              </div>
            </fieldset>

            <fieldset class="space-y-4">
              <legend class="mb-3 text-sm font-semibold">Alert thresholds</legend>
              <p class="text-xs text-muted-foreground">
                Usage levels, from 1 to 100 percent, that trigger an alert.
              </p>
              <div class="grid gap-4 sm:grid-cols-3">
                {percentField("alert-cpu", "CPU", "cpu")}
                {percentField("alert-memory", "Memory", "memory")}
                {percentField("alert-disk", "Disk", "disk")}
              </div>
            </fieldset>

            <Show when={save.isError}>
              <Alert variant="destructive">
                <AlertDescription>{describeError(save.error)}</AlertDescription>
              </Alert>
            </Show>

            <div class="flex flex-wrap items-center gap-3">
              <Button type="submit" disabled={!canSave()}>
                {save.isPending ? "Saving…" : "Save"}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={!value().email.enabled || testEmail.isPending}
                onClick={() => testEmail.mutate(undefined)}
              >
                {testEmail.isPending ? "Sending…" : "Send test email"}
              </Button>
              <Show when={testEmail.isSuccess}>
                <span class="text-sm text-success">
                  Test email sent to {settings.data?.email.to}
                </span>
              </Show>
              <Show when={testEmail.isError}>
                <span class="text-sm text-destructive">{describeError(testEmail.error)}</span>
              </Show>
            </div>
          </form>
        </CardContent>
      </Card>
    </Show>
  );
}
