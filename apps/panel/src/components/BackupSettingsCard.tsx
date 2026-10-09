import { createSignal, Show } from "solid-js";
import { useSaveBackups, useSettingsBackups } from "@/api/queries";
import { describeError } from "@/api/client";
import type { BackupSettings } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { CardSkeleton } from "@/components/CardSkeleton";

const DEFAULTS: BackupSettings = { schedule_hour: 3, retention_days: 7 };

/** Parses a number field. An empty field is NaN, which fails validation. */
const parseField = (text: string) => (text.trim() === "" ? Number.NaN : Number(text));
const inRange = (n: number, low: number, high: number) =>
  Number.isInteger(n) && n >= low && n <= high;

/** Administrators choose when the daily website backups run and how long they are kept. */
export function BackupSettingsCard() {
  const settings = useSettingsBackups();
  const save = useSaveBackups();
  const [draft, setDraft] = createSignal<BackupSettings | null>(null);

  const value = (): BackupSettings => draft() ?? settings.data ?? DEFAULTS;
  const dirty = () =>
    settings.data !== undefined && JSON.stringify(value()) !== JSON.stringify(settings.data);
  const hourValid = () => inRange(value().schedule_hour, 0, 23);
  const retentionValid = () => inRange(value().retention_days, 1, 90);
  const canSave = () => dirty() && hourValid() && retentionValid() && !save.isPending;
  const set = (patch: Partial<BackupSettings>) => setDraft({ ...value(), ...patch });
  const clock = () => (hourValid() ? `${String(value().schedule_hour).padStart(2, "0")}:00` : "—");

  const submit = (event: SubmitEvent) => {
    event.preventDefault();
    if (!canSave()) return;
    save.mutate(value(), { onSuccess: () => setDraft(null) });
  };

  return (
    <Show when={!settings.isPending} fallback={<CardSkeleton height="h-56" />}>
      <Card>
        <CardHeader>
          <CardTitle>Backups</CardTitle>
          <CardDescription>
            When websites are backed up each day, and how many days of backups are kept.
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
