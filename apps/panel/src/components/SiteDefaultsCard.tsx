import { Select } from "@/components/ui/select";
import { createSignal, For, Show } from "solid-js";
import { useSaveDefaults, usePHPVersions, useSettingsDefaults } from "@/api/queries";
import { describeError } from "@/api/client";
import type { SiteDefaults } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Label } from "@/components/ui/label";
import { CardSkeleton } from "@/components/CardSkeleton";

/** The five newest PHP releases a website can run on. */
const PHP_CHOICES = ["8.5", "8.4", "8.3", "8.2", "8.1"];

/** Administrators choose the PHP version that new websites start with. */
export function SiteDefaultsCard() {
  const defaults = useSettingsDefaults();
  const installed = usePHPVersions();
  const save = useSaveDefaults();
  const [draft, setDraft] = createSignal<SiteDefaults | null>(null);

  const value = (): SiteDefaults => draft() ?? defaults.data ?? { php_version: "" };
  const dirty = () =>
    defaults.data !== undefined && JSON.stringify(value()) !== JSON.stringify(defaults.data);
  const isInstalled = (version: string) => (installed.data ?? []).includes(version);
  const canSave = () => dirty() && isInstalled(value().php_version) && !save.isPending;

  const submit = (event: SubmitEvent) => {
    event.preventDefault();
    if (!canSave()) return;
    save.mutate(value(), { onSuccess: () => setDraft(null) });
  };

  return (
    <Show when={!defaults.isPending} fallback={<CardSkeleton height="h-48" />}>
      <Card>
        <CardHeader>
          <CardTitle>Site defaults</CardTitle>
          <CardDescription>The PHP version that new websites start with.</CardDescription>
        </CardHeader>
        <CardContent>
          <form class="space-y-4" onSubmit={submit} noValidate>
            <Show when={defaults.isError}>
              <Alert variant="destructive">
                <AlertDescription>{describeError(defaults.error)}</AlertDescription>
              </Alert>
            </Show>

            <div class="max-w-xs space-y-1.5">
              <Label for="default-php">PHP version</Label>
              <Select
                id="default-php"
                onChange={(e) => setDraft({ php_version: e.currentTarget.value })}
              >
                <For each={PHP_CHOICES}>
                  {(v) => (
                    <option
                      value={v}
                      selected={v === value().php_version}
                      disabled={!isInstalled(v)}
                    >
                      PHP {v}
                      {isInstalled(v) ? "" : " (not installed on this server)"}
                    </option>
                  )}
                </For>
              </Select>
              <p class="text-xs text-muted-foreground">
                Versions that are not installed on this server cannot be chosen.
              </p>
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
