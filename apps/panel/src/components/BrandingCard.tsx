import { useState, type FormEvent } from "react";
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
  const current = branding.data;
  const [draft, setDraft] = useState<Branding | null>(null);
  const value: Branding = draft ?? current ?? { name: "", tagline: "", primary_color: "#3b6fd4" };
  const dirty =
    draft !== null && current !== undefined && JSON.stringify(draft) !== JSON.stringify(current);
  const colorValid = HEX.test(value.primary_color);
  const nameValid = value.name.trim().length >= 2 && value.name.trim().length <= 40;

  const set = (patch: Partial<Branding>) => setDraft({ ...value, ...patch });

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!dirty || !colorValid || !nameValid) return;
    save.mutate(
      { ...value, name: value.name.trim(), tagline: value.tagline.trim() },
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
        <form onSubmit={submit} className="grid gap-6 lg:grid-cols-[1fr_minmax(0,1fr)]" noValidate>
          <div className="space-y-5">
            <div className="space-y-2">
              <Label htmlFor="brand-name">Panel name</Label>
              <Input
                id="brand-name"
                value={value.name}
                maxLength={40}
                onChange={(e) => set({ name: e.target.value })}
                aria-invalid={!nameValid || undefined}
              />
              <p className="text-xs text-muted-foreground">2 to 40 characters.</p>
            </div>

            <div className="space-y-2">
              <Label htmlFor="brand-tagline">Tagline</Label>
              <Input
                id="brand-tagline"
                value={value.tagline}
                maxLength={120}
                onChange={(e) => set({ tagline: e.target.value })}
              />
            </div>

            <div className="space-y-2">
              <Label htmlFor="brand-color">Accent colour</Label>
              <div className="flex flex-wrap items-center gap-2">
                <input
                  id="brand-color"
                  type="color"
                  value={colorValid ? value.primary_color : "#3b6fd4"}
                  onChange={(e) => set({ primary_color: e.target.value })}
                  className="h-10 w-14 cursor-pointer rounded-md border border-input bg-background p-1"
                  aria-label="Pick accent colour"
                />
                <Input
                  className="w-32 font-mono"
                  value={value.primary_color}
                  onChange={(e) => set({ primary_color: e.target.value })}
                  aria-invalid={!colorValid || undefined}
                  aria-label="Accent colour as hex"
                />
              </div>
              <div className="flex flex-wrap gap-2 pt-1" role="group" aria-label="Preset colours">
                {ACCENT_PRESETS.map((c) => (
                  <button
                    key={c}
                    type="button"
                    onClick={() => set({ primary_color: c })}
                    aria-label={`Use ${c}`}
                    aria-pressed={value.primary_color.toLowerCase() === c}
                    className={cn(
                      "flex size-8 items-center justify-center rounded-full ring-offset-2 ring-offset-card transition",
                      value.primary_color.toLowerCase() === c && "ring-2 ring-foreground",
                    )}
                    style={{ background: c }}
                  >
                    {value.primary_color.toLowerCase() === c && (
                      <Check className="size-4" style={{ color: foregroundFor(c) }} aria-hidden />
                    )}
                  </button>
                ))}
              </div>
            </div>

            {save.isError && (
              <Alert variant="destructive">
                <AlertDescription>{describeError(save.error)}</AlertDescription>
              </Alert>
            )}

            <div className="flex gap-2">
              <Button
                type="submit"
                disabled={!dirty || !colorValid || !nameValid || save.isPending}
              >
                {save.isPending ? "Saving…" : "Save branding"}
              </Button>
              {dirty && (
                <Button type="button" variant="ghost" onClick={() => setDraft(null)}>
                  Discard
                </Button>
              )}
            </div>
          </div>

          <div aria-label="Preview" className="space-y-3">
            <p className="text-xs font-medium uppercase tracking-wide text-muted-foreground">
              Preview
            </p>
            <div className="overflow-hidden rounded-xl border border-border bg-background">
              <div className="flex items-center gap-2.5 border-b border-border px-4 py-3">
                <span
                  className="flex size-8 items-center justify-center rounded-lg text-sm font-bold"
                  style={{
                    background: colorValid ? value.primary_color : "#3b6fd4",
                    color: foregroundFor(colorValid ? value.primary_color : "#3b6fd4"),
                  }}
                >
                  {(value.name.trim()[0] ?? "Z").toUpperCase()}
                </span>
                <span className="font-semibold tracking-tight">
                  {value.name.trim() || "Your panel"}
                </span>
              </div>
              <div className="space-y-3 p-5">
                <p className="text-lg font-semibold tracking-tight">
                  {value.tagline.trim() || "Your tagline"}
                </p>
                <p className="text-sm text-muted-foreground">
                  How buttons and highlights will look.
                </p>
                <button
                  type="button"
                  className="h-9 rounded-md px-4 text-sm font-medium"
                  style={{
                    background: colorValid ? value.primary_color : "#3b6fd4",
                    color: foregroundFor(colorValid ? value.primary_color : "#3b6fd4"),
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
