import { createSignal, For, Show } from "solid-js";
import { Check } from "@/components/icons";
import { useBranding, useSaveBranding } from "@/api/queries";
import { describeError } from "@/api/client";
import type { Branding } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { ACCENT_PRESETS, foregroundFor } from "@/lib/brand";
import { cn } from "@/lib/utils";

const HEX = /^#[0-9a-fA-F]{6}$/;

/** Administrators choose the panel's name, tagline and accent colour. */
export function BrandingCard() {
  const branding = useBranding();
  const save = useSaveBranding();
  const [draft, setDraft] = createSignal<Branding | null>(null);

  const value = (): Branding =>
    draft() ?? branding.data ?? { name: "", tagline: "", primary_color: "#3b6fd4" };
  const dirty = () =>
    draft() !== null &&
    branding.data !== undefined &&
    JSON.stringify(draft()) !== JSON.stringify(branding.data);
  const colorValid = () => HEX.test(value().primary_color);
  const nameValid = () => value().name.trim().length >= 2 && value().name.trim().length <= 40;
  const previewColor = () => (colorValid() ? value().primary_color : "#3b6fd4");

  const set = (patch: Partial<Branding>) => setDraft({ ...value(), ...patch });

  const submit = (event: SubmitEvent) => {
    event.preventDefault();
    if (!dirty() || !colorValid() || !nameValid()) return;
    const v = value();
    save.mutate(
      { ...v, name: v.name.trim(), tagline: v.tagline.trim() },
      { onSuccess: () => setDraft(null) },
    );
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Branding</CardTitle>
        <CardDescription>
          Name, tagline and accent colour shown on the sign-in page and across the panel.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={submit} class="grid gap-6 lg:grid-cols-[1fr_minmax(0,1fr)]" noValidate>
          <div class="space-y-5">
            <div class="space-y-2">
              <Label for="brand-name">Panel name</Label>
              <Input
                id="brand-name"
                value={value().name}
                maxLength={40}
                onInput={(e) => set({ name: e.currentTarget.value })}
                aria-invalid={!nameValid() || undefined}
              />
              <p class="text-xs text-muted-foreground">2 to 40 characters.</p>
            </div>

            <div class="space-y-2">
              <Label for="brand-tagline">Tagline</Label>
              <Input
                id="brand-tagline"
                value={value().tagline}
                maxLength={120}
                onInput={(e) => set({ tagline: e.currentTarget.value })}
              />
            </div>

            <div class="space-y-2">
              <Label for="brand-color">Accent colour</Label>
              <div class="flex flex-wrap items-center gap-2">
                <input
                  id="brand-color"
                  type="color"
                  value={colorValid() ? value().primary_color : "#3b6fd4"}
                  onInput={(e) => set({ primary_color: e.currentTarget.value })}
                  class="h-10 w-14 cursor-pointer rounded-md border border-input bg-background p-1"
                  aria-label="Pick accent colour"
                />
                <Input
                  class="w-32 font-mono"
                  value={value().primary_color}
                  onInput={(e) => set({ primary_color: e.currentTarget.value })}
                  aria-invalid={!colorValid() || undefined}
                  aria-label="Accent colour as hex"
                />
              </div>
              <div class="flex flex-wrap gap-2 pt-1" role="group" aria-label="Preset colours">
                <For each={ACCENT_PRESETS}>
                  {(c) => (
                    <button
                      type="button"
                      onClick={() => set({ primary_color: c })}
                      aria-label={`Use ${c}`}
                      aria-pressed={value().primary_color.toLowerCase() === c}
                      class={cn(
                        "flex size-8 items-center justify-center rounded-full ring-offset-2 ring-offset-card transition",
                        value().primary_color.toLowerCase() === c && "ring-2 ring-foreground",
                      )}
                      style={{ background: c }}
                    >
                      <Show when={value().primary_color.toLowerCase() === c}>
                        <Check
                          class="size-4"
                          style={{ color: foregroundFor(c) }}
                          aria-hidden="true"
                        />
                      </Show>
                    </button>
                  )}
                </For>
              </div>
            </div>

            <Show when={save.isError}>
              <Alert variant="destructive">
                <AlertDescription>{describeError(save.error)}</AlertDescription>
              </Alert>
            </Show>

            <div class="flex gap-2">
              <Button
                type="submit"
                disabled={!dirty() || !colorValid() || !nameValid() || save.isPending}
              >
                {save.isPending ? "Saving…" : "Save branding"}
              </Button>
              <Show when={dirty()}>
                <Button type="button" variant="ghost" onClick={() => setDraft(null)}>
                  Discard
                </Button>
              </Show>
            </div>
          </div>

          <div aria-label="Preview" class="space-y-3">
            <p class="text-xs font-medium uppercase tracking-wide text-muted-foreground">Preview</p>
            <div class="overflow-hidden rounded-xl border border-border bg-background">
              <div class="flex items-center gap-2.5 border-b border-border px-4 py-3">
                <span
                  class="flex size-8 items-center justify-center rounded-lg text-sm font-bold"
                  style={{
                    background: previewColor(),
                    color: foregroundFor(previewColor()),
                  }}
                >
                  {(value().name.trim()[0] ?? "Z").toUpperCase()}
                </span>
                <span class="font-semibold tracking-tight">
                  {value().name.trim() || "Your panel"}
                </span>
              </div>
              <div class="space-y-3 p-5">
                <p class="text-lg font-semibold tracking-tight">
                  {value().tagline.trim() || "Your tagline"}
                </p>
                <p class="text-sm text-muted-foreground">How buttons and highlights will look.</p>
                <button
                  type="button"
                  class="h-9 rounded-md px-4 text-sm font-medium"
                  style={{
                    background: previewColor(),
                    color: foregroundFor(previewColor()),
                  }}
                >
                  Primary action
                </button>
              </div>
            </div>
          </div>
        </form>
      </CardContent>
    </Card>
  );
}
