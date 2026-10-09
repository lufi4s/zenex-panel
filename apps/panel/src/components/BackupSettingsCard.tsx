import { createSignal, Show } from "solid-js";
import {
  useSaveBackups,
  useSettingsBackups,
  useSftpKey,
  useSftpPublicKey,
  useTestSftp,
} from "@/api/queries";
import { ApiError, describeError } from "@/api/client";
import type { BackupSettings, SftpSettings } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CardSkeleton } from "@/components/CardSkeleton";
import { Check, Copy } from "@/components/icons";

const DEFAULT_SFTP: SftpSettings = { host: "", port: 22, username: "", path: "/backups/zenex" };
const DEFAULTS: BackupSettings = {
  schedule_hour: 3,
  retention_days: 7,
  destination: { type: "local", sftp: DEFAULT_SFTP },
};

/** Fills in fields an older API response may not include. */
function withDefaults(s: BackupSettings): BackupSettings {
  return {
    schedule_hour: s.schedule_hour,
    retention_days: s.retention_days,
    destination: {
      type: s.destination?.type === "sftp" ? "sftp" : "local",
      sftp: { ...DEFAULT_SFTP, ...s.destination?.sftp },
    },
  };
}

/** Parses a number field. An empty field is NaN, which fails validation. */
const parseField = (text: string) => (text.trim() === "" ? Number.NaN : Number(text));
const inRange = (n: number, low: number, high: number) =>
  Number.isInteger(n) && n >= low && n <= high;

/** Trims the text fields so the API never gets stray spaces. */
function cleanSftp(s: SftpSettings): SftpSettings {
  return { host: s.host.trim(), port: s.port, username: s.username.trim(), path: s.path.trim() };
}

/** Administrators choose when the daily website backups run, how long they are kept, and where. */
export function BackupSettingsCard() {
  const settings = useSettingsBackups();
  const save = useSaveBackups();
  const sftpKey = useSftpKey();
  const generateKey = useSftpPublicKey();
  const testSftp = useTestSftp();
  const [draft, setDraft] = createSignal<BackupSettings | null>(null);
  const [copied, setCopied] = createSignal(false);

  const baseline = (): BackupSettings => (settings.data ? withDefaults(settings.data) : DEFAULTS);
  const value = (): BackupSettings => draft() ?? baseline();
  const dirty = () =>
    settings.data !== undefined && JSON.stringify(value()) !== JSON.stringify(baseline());
  const hourValid = () => inRange(value().schedule_hour, 0, 23);
  const retentionValid = () => inRange(value().retention_days, 1, 90);
  const isSftp = () => value().destination.type === "sftp";

  const sftp = () => value().destination.sftp;
  const hostValid = () => sftp().host.trim().length > 0 && sftp().host.trim().length <= 255;
  const portValid = () => inRange(sftp().port, 1, 65535);
  const usernameValid = () =>
    sftp().username.trim().length > 0 && sftp().username.trim().length <= 64;
  const pathValid = () => sftp().path.trim().startsWith("/") && sftp().path.trim().length <= 255;
  const sftpValid = () =>
    !isSftp() || (hostValid() && portValid() && usernameValid() && pathValid());

  const canSave = () =>
    dirty() && hourValid() && retentionValid() && sftpValid() && !save.isPending;
  const set = (patch: Partial<BackupSettings>) => setDraft({ ...value(), ...patch });
  const setSftp = (patch: Partial<SftpSettings>) => {
    testSftp.reset();
    set({ destination: { ...value().destination, sftp: { ...sftp(), ...patch } } });
  };
  const setType = (type: "local" | "sftp") => {
    testSftp.reset();
    set({ destination: { ...value().destination, type } });
  };
  const clock = () => (hourValid() ? `${String(value().schedule_hour).padStart(2, "0")}:00` : "—");

  const publicKey = () => sftpKey.data?.public_key ?? "";
  const keyMissing = () =>
    sftpKey.error instanceof ApiError && sftpKey.error.status === 404 && !publicKey();

  const submit = (event: SubmitEvent) => {
    event.preventDefault();
    if (!canSave()) return;
    save.mutate(
      {
        schedule_hour: value().schedule_hour,
        retention_days: value().retention_days,
        destination: {
          type: value().destination.type,
          sftp: cleanSftp(sftp()),
        },
      },
      { onSuccess: () => setDraft(null) },
    );
  };

  const copyKey = async () => {
    try {
      await navigator.clipboard.writeText(publicKey());
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2_000);
    } catch {
      setCopied(false);
    }
  };

  return (
    <Show when={!settings.isPending} fallback={<CardSkeleton height="h-56" />}>
      <Card>
        <CardHeader>
          <CardTitle>Backups</CardTitle>
          <CardDescription>
            When websites are backed up each day, how many days of backups are kept, and where they
            are stored.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form class="space-y-5" onSubmit={submit} noValidate>
            <Show when={settings.isError}>
              <Alert variant="destructive">
                <AlertDescription>{describeError(settings.error)}</AlertDescription>
              </Alert>
            </Show>

            <div class="grid gap-5 sm:grid-cols-2">
              <div class="space-y-2">
                <Label for="backup-hour">Daily backup hour (0 to 23)</Label>
                <div class="flex items-center gap-3">
                  <Input
                    id="backup-hour"
                    type="number"
                    min={0}
                    max={23}
                    step={1}
                    class="w-24"
                    value={Number.isNaN(value().schedule_hour) ? "" : value().schedule_hour}
                    onInput={(e) => set({ schedule_hour: parseField(e.currentTarget.value) })}
                    aria-invalid={!hourValid() || undefined}
                  />
                  <span class="font-mono text-sm text-muted-foreground">{clock()}</span>
                </div>
                <p class="text-xs text-muted-foreground">Server time.</p>
              </div>

              <div class="space-y-2">
                <Label for="backup-retention">Keep backups for (days)</Label>
                <Input
                  id="backup-retention"
                  type="number"
                  min={1}
                  max={90}
                  step={1}
                  class="w-24"
                  value={Number.isNaN(value().retention_days) ? "" : value().retention_days}
                  onInput={(e) => set({ retention_days: parseField(e.currentTarget.value) })}
                  aria-invalid={!retentionValid() || undefined}
                />
                <p class="text-xs text-muted-foreground">1 to 90 days.</p>
              </div>
            </div>

            <fieldset class="space-y-3">
              <legend class="mb-2 text-sm font-medium leading-none">Where to store backups</legend>
              <div class="flex flex-wrap gap-6">
                <label class="flex items-center gap-2 text-sm">
                  <input
                    type="radio"
                    name="backup-destination"
                    value="local"
                    class="size-4 accent-primary"
                    checked={value().destination.type === "local"}
                    onChange={() => setType("local")}
                  />
                  This server
                </label>
                <label class="flex items-center gap-2 text-sm">
                  <input
                    type="radio"
                    name="backup-destination"
                    value="sftp"
                    class="size-4 accent-primary"
                    checked={value().destination.type === "sftp"}
                    onChange={() => setType("sftp")}
                  />
                  Remote server (SFTP)
                </label>
              </div>
            </fieldset>

            <Show when={isSftp()}>
              <div class="space-y-5 rounded-lg border border-border p-4">
                <p class="text-xs text-muted-foreground">
                  Use SFTP (SSH file transfer). Add the public key below to the remote server's
                  ~/.ssh/authorized_keys for this user.
                </p>

                <div class="grid gap-4 sm:grid-cols-2">
                  <div class="space-y-2">
                    <Label for="backup-sftp-host">Host</Label>
                    <Input
                      id="backup-sftp-host"
                      autocomplete="off"
                      value={sftp().host}
                      onInput={(e) => setSftp({ host: e.currentTarget.value })}
                      aria-invalid={!hostValid() || undefined}
                    />
                  </div>
                  <div class="space-y-2">
                    <Label for="backup-sftp-port">Port</Label>
                    <Input
                      id="backup-sftp-port"
                      type="number"
                      min={1}
                      max={65535}
                      step={1}
                      class="w-28"
                      value={Number.isNaN(sftp().port) ? "" : sftp().port}
                      onInput={(e) => setSftp({ port: parseField(e.currentTarget.value) })}
                      aria-invalid={!portValid() || undefined}
                    />
                  </div>
                  <div class="space-y-2">
                    <Label for="backup-sftp-username">Username</Label>
                    <Input
                      id="backup-sftp-username"
                      autocomplete="off"
                      value={sftp().username}
                      onInput={(e) => setSftp({ username: e.currentTarget.value })}
                      aria-invalid={!usernameValid() || undefined}
                    />
                  </div>
                  <div class="space-y-2">
                    <Label for="backup-sftp-path">Remote folder</Label>
                    <Input
                      id="backup-sftp-path"
                      autocomplete="off"
                      value={sftp().path}
                      onInput={(e) => setSftp({ path: e.currentTarget.value })}
                      aria-invalid={!pathValid() || undefined}
                    />
                    <p class="text-xs text-muted-foreground">Must start with /.</p>
                  </div>
                </div>

                <div class="space-y-2">
                  <Label for="backup-sftp-key">Public key</Label>
                  <Show when={publicKey()}>
                    <textarea
                      id="backup-sftp-key"
                      readOnly
                      rows={3}
                      class="w-full rounded-md border border-input bg-background px-3 py-2 font-mono text-xs"
                      value={publicKey()}
                    />
                    <Button type="button" variant="outline" size="sm" onClick={copyKey}>
                      {copied() ? <Check aria-hidden="true" /> : <Copy aria-hidden="true" />}
                      {copied() ? "Copied" : "Copy"}
                    </Button>
                  </Show>
                  <Show when={!publicKey() && keyMissing()}>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={generateKey.isPending}
                      onClick={() => generateKey.mutate(undefined)}
                    >
                      {generateKey.isPending ? "Generating…" : "Generate key"}
                    </Button>
                  </Show>
                  <Show when={!publicKey() && !keyMissing() && !sftpKey.isPending}>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      disabled={generateKey.isPending}
                      onClick={() => generateKey.mutate(undefined)}
                    >
                      {generateKey.isPending ? "Loading…" : "Show public key"}
                    </Button>
                  </Show>
                  <Show when={generateKey.isError}>
                    <Alert variant="destructive">
                      <AlertDescription>{describeError(generateKey.error)}</AlertDescription>
                    </Alert>
                  </Show>
                </div>

                <div class="flex flex-wrap items-center gap-3">
                  <Button
                    type="button"
                    variant="outline"
                    disabled={!sftpValid() || testSftp.isPending}
                    onClick={() => testSftp.mutate(cleanSftp(sftp()))}
                  >
                    {testSftp.isPending ? "Testing…" : "Test connection"}
                  </Button>
                  <Show when={testSftp.isSuccess}>
                    <span class="text-sm text-success">Connection works</span>
                  </Show>
                  <Show when={testSftp.isError}>
                    <span class="text-sm text-destructive">{describeError(testSftp.error)}</span>
                  </Show>
                </div>
              </div>
            </Show>

            <Show when={save.isError}>
              <Alert variant="destructive">
                <AlertDescription>{describeError(save.error)}</AlertDescription>
              </Alert>
            </Show>

            <Button type="submit" disabled={!canSave()}>
              {save.isPending ? "Saving…" : "Save"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </Show>
  );
}
