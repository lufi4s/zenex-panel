import { createSignal, For, Show } from "solid-js";
import {
  useRunBackupsNow,
  useSaveBackups,
  useSettingsBackups,
  useSftpKey,
  useSftpPublicKey,
  useTestSftp,
} from "@/api/queries";
import { ApiError, describeError } from "@/api/client";
import { Link } from "@/lib/router";
import type { BackupFrequency, BackupSettings, SftpAuth, SftpSettings } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CardSkeleton } from "@/components/CardSkeleton";
import { Check, Copy } from "@/components/icons";

const DEFAULT_SFTP: SftpSettings = {
  host: "",
  port: 22,
  username: "",
  path: "/backups/zenex",
  auth: "key",
  password_set: false,
};
const DEFAULTS: BackupSettings = {
  frequency: "daily",
  schedule_hour: 3,
  weekday: 0,
  retention_days: 7,
  destination: { type: "local", sftp: DEFAULT_SFTP },
};

const HOURS = Array.from({ length: 24 }, (_, hour) => hour);
const WEEKDAYS = ["Sunday", "Monday", "Tuesday", "Wednesday", "Thursday", "Friday", "Saturday"];
const SELECT_CLASS =
  "flex h-10 w-full rounded-md border border-input bg-card px-3 text-base sm:h-9 sm:text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/30";
/** Hourly backups above this many days of retention get a space warning. */
const HOURLY_RETENTION_WARNING_DAYS = 14;

/** Fills in fields an older API response may not include. */
function withDefaults(s: BackupSettings): BackupSettings {
  return {
    frequency: s.frequency ?? DEFAULTS.frequency,
    schedule_hour: s.schedule_hour,
    weekday: s.weekday ?? DEFAULTS.weekday,
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

/** Trims the text fields so the API never gets stray spaces. Never includes the typed password. */
function cleanSftp(s: SftpSettings): SftpSettings {
  return {
    host: s.host.trim(),
    port: s.port,
    username: s.username.trim(),
    path: s.path.trim(),
    auth: s.auth,
    password_set: s.password_set,
  };
}

/** Administrators choose when the daily website backups run, how long they are kept, and where. */
export function BackupSettingsCard() {
  const settings = useSettingsBackups();
  const save = useSaveBackups();
  const runNow = useRunBackupsNow();
  const sftpKey = useSftpKey();
  const generateKey = useSftpPublicKey();
  const testSftp = useTestSftp();
  const [draft, setDraft] = createSignal<BackupSettings | null>(null);
  const [copied, setCopied] = createSignal(false);
  const [passwordError, setPasswordError] = createSignal(false);

  const baseline = (): BackupSettings => (settings.data ? withDefaults(settings.data) : DEFAULTS);
  const value = (): BackupSettings => draft() ?? baseline();
  const dirty = () =>
    settings.data !== undefined && JSON.stringify(value()) !== JSON.stringify(baseline());
  const hourValid = () => inRange(value().schedule_hour, 0, 23);
  const frequency = () => value().frequency;
  const setFrequency = (next: BackupFrequency) => set({ frequency: next });
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
  const isPasswordAuth = () => isSftp() && sftp().auth === "password";
  /** Password sign-in with no saved password and nothing typed yet. */
  const passwordMissing = () =>
    isPasswordAuth() && !sftp().password_set && (sftp().password ?? "") === "";

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
  const setAuth = (auth: SftpAuth) => {
    setPasswordError(false);
    setSftp({ auth });
  };
  const setPassword = (password: string) => {
    setPasswordError(false);
    setSftp({ password });
  };
  const timeText = () => `${String(value().schedule_hour).padStart(2, "0")}:00`;
  /** Plain-words description of the next run, shown under the schedule fields. */
  const nextRunText = () => {
    if (frequency() === "hourly") return "Next backup: every hour, on the hour.";
    if (frequency() === "weekly") {
      return `Next backup: every ${WEEKDAYS[value().weekday] ?? WEEKDAYS[0]} at ${timeText()}.`;
    }
    return `Next backup: every day at ${timeText()}.`;
  };
  const runNowMessage = () => {
    const result = runNow.data;
    if (!result) return "";
    const skipped = result.skipped > 0 ? ` (${result.skipped} already running)` : "";
    return `Started backups for ${result.started} sites.${skipped}`;
  };

  const publicKey = () => sftpKey.data?.public_key ?? "";
  const keyMissing = () =>
    sftpKey.error instanceof ApiError && sftpKey.error.status === 404 && !publicKey();

  const submit = (event: SubmitEvent) => {
    event.preventDefault();
    if (!canSave()) return;
    if (passwordMissing()) {
      setPasswordError(true);
      return;
    }
    const typed = sftp().password ?? "";
    const sftpBody: SftpSettings =
      isPasswordAuth() && typed !== ""
        ? { ...cleanSftp(sftp()), password: typed }
        : cleanSftp(sftp());
    save.mutate(
      {
        frequency: value().frequency,
        schedule_hour: value().schedule_hour,
        weekday: value().weekday,
        retention_days: value().retention_days,
        destination: {
          type: value().destination.type,
          sftp: sftpBody,
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

  const keyBlock = () => (
    <div class="space-y-3">
      <p class="text-xs text-muted-foreground">
        Add the public key below to the remote server's ~/.ssh/authorized_keys for this user.
      </p>
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
    </div>
  );

  return (
    <Show when={!settings.isPending} fallback={<CardSkeleton height="h-56" />}>
      <Card>
        <CardHeader>
          <CardTitle>Backups</CardTitle>
          <CardDescription>
            When websites are backed up, how many days of backups are kept, and where they are
            stored.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <form class="space-y-5" onSubmit={submit} noValidate>
            <Show when={settings.isError}>
              <Alert variant="destructive">
                <AlertDescription>{describeError(settings.error)}</AlertDescription>
              </Alert>
            </Show>

            <fieldset class="space-y-3">
              <legend class="mb-2 text-sm font-medium leading-none">How often</legend>
              <div class="flex flex-wrap gap-6">
                <label class="flex items-center gap-2 text-sm">
                  <input
                    type="radio"
                    name="backup-frequency"
                    value="hourly"
                    class="size-4 accent-primary"
                    checked={frequency() === "hourly"}
                    onChange={() => setFrequency("hourly")}
                  />
                  Every hour
                </label>
                <label class="flex items-center gap-2 text-sm">
                  <input
                    type="radio"
                    name="backup-frequency"
                    value="daily"
                    class="size-4 accent-primary"
                    checked={frequency() === "daily"}
                    onChange={() => setFrequency("daily")}
                  />
                  Every day
                </label>
                <label class="flex items-center gap-2 text-sm">
                  <input
                    type="radio"
                    name="backup-frequency"
                    value="weekly"
                    class="size-4 accent-primary"
                    checked={frequency() === "weekly"}
                    onChange={() => setFrequency("weekly")}
                  />
                  Every week
                </label>
              </div>
            </fieldset>

            <div class="grid gap-5 sm:grid-cols-2">
              <Show when={frequency() === "hourly"}>
                <p class="text-xs text-muted-foreground sm:col-span-2">
                  Runs at the top of every hour.
                </p>
              </Show>

              <Show when={frequency() === "weekly"}>
                <div class="space-y-2">
                  <Label for="backup-weekday">Day</Label>
                  <select
                    id="backup-weekday"
                    class={SELECT_CLASS}
                    onChange={(e) => set({ weekday: Number(e.currentTarget.value) })}
                  >
                    <For each={WEEKDAYS}>
                      {(name, index) => (
                        <option value={String(index())} selected={index() === value().weekday}>
                          {name}
                        </option>
                      )}
                    </For>
                  </select>
                </div>
              </Show>

              <Show when={frequency() !== "hourly"}>
                <div class="space-y-2">
                  <Label for="backup-hour">Time</Label>
                  <select
                    id="backup-hour"
                    class={SELECT_CLASS}
                    onChange={(e) => set({ schedule_hour: Number(e.currentTarget.value) })}
                  >
                    <For each={HOURS}>
                      {(hour) => (
                        <option value={String(hour)} selected={hour === value().schedule_hour}>
                          {`${String(hour).padStart(2, "0")}:00`}
                        </option>
                      )}
                    </For>
                  </select>
                  <p class="text-xs text-muted-foreground">Server time.</p>
                </div>
              </Show>

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
                <Show
                  when={
                    frequency() === "hourly" &&
                    value().retention_days > HOURLY_RETENTION_WARNING_DAYS
                  }
                >
                  <p class="text-xs text-warning">
                    Hourly backups use a lot of space. Consider a shorter retention.
                  </p>
                </Show>
              </div>
            </div>

            <p class="text-sm text-muted-foreground">{nextRunText()}</p>

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
                  Use SFTP (SSH file transfer). Choose how the panel signs in to the remote server.
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

                <fieldset class="space-y-3">
                  <legend class="mb-2 text-sm font-medium leading-none">Sign in with</legend>
                  <div class="flex flex-wrap gap-6">
                    <label class="flex items-center gap-2 text-sm">
                      <input
                        type="radio"
                        name="backup-sftp-auth"
                        value="key"
                        class="size-4 accent-primary"
                        checked={sftp().auth === "key"}
                        onChange={() => setAuth("key")}
                      />
                      SSH key
                    </label>
                    <label class="flex items-center gap-2 text-sm">
                      <input
                        type="radio"
                        name="backup-sftp-auth"
                        value="password"
                        class="size-4 accent-primary"
                        checked={sftp().auth === "password"}
                        onChange={() => setAuth("password")}
                      />
                      Password
                    </label>
                  </div>
                </fieldset>

                <Show when={!isPasswordAuth()}>{keyBlock()}</Show>

                <Show when={isPasswordAuth()}>
                  <div class="space-y-2">
                    <Label for="backup-sftp-password">SFTP password</Label>
                    <Input
                      id="backup-sftp-password"
                      type="password"
                      autocomplete="new-password"
                      placeholder={
                        sftp().password_set ? "Leave blank to keep the saved password" : undefined
                      }
                      value={sftp().password ?? ""}
                      onInput={(e) => setPassword(e.currentTarget.value)}
                      aria-invalid={(passwordError() && passwordMissing()) || undefined}
                    />
                    <Show when={passwordError() && passwordMissing()}>
                      <p class="text-xs text-destructive">Enter the SFTP password.</p>
                    </Show>
                  </div>
                  <details class="rounded-md border border-border p-3">
                    <summary class="cursor-pointer text-sm">Also keep the key ready</summary>
                    <div class="mt-3">{keyBlock()}</div>
                  </details>
                </Show>

                <div class="flex flex-wrap items-center gap-3">
                  <Button
                    type="button"
                    variant="outline"
                    disabled={!sftpValid() || testSftp.isPending}
                    onClick={() => testSftp.mutate(cleanSftp(sftp()))}
                  >
                    {testSftp.isPending ? "Testing…" : "Test connection"}
                  </Button>
                  <p class="text-xs text-muted-foreground">
                    Save first, then test. The test uses the saved settings.
                  </p>
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

            <div class="flex flex-wrap items-center gap-3">
              <Button type="submit" disabled={!canSave()}>
                {save.isPending ? "Saving…" : "Save"}
              </Button>
              <Button
                type="button"
                variant="outline"
                disabled={runNow.isPending}
                onClick={() => runNow.mutate(undefined)}
              >
                {runNow.isPending ? "Starting…" : "Back up now"}
              </Button>
              <Show when={runNow.isSuccess}>
                <span class="text-sm text-success">{runNowMessage()}</span>
                <p class="basis-full text-sm text-muted-foreground">
                  Watch progress on each website&apos;s Settings tab.{" "}
                  <Link to="/websites" class="underline underline-offset-4 hover:text-foreground">
                    Go to websites
                  </Link>
                </p>
              </Show>
              <Show when={runNow.isError}>
                <span class="text-sm text-destructive">{describeError(runNow.error)}</span>
              </Show>
            </div>
          </form>
        </CardContent>
      </Card>
    </Show>
  );
}
