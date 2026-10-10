import { Show, For } from "solid-js";
import { useDomains, useSettingsAlerts, useSites } from "@/api/queries";
import { ArrowRight, Check, CircleDashed } from "@/components/icons";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Link } from "@/lib/router";

type Step = { label: string; done: boolean; to: string };

/**
 * A checklist for a new panel: each step ticks itself when the panel can see it done.
 * Hidden once every step is complete. The alert step only applies to administrators.
 */
export function GettingStarted(props: { isAdmin: boolean }) {
  const domains = useDomains();
  const sites = useSites();
  const alerts = useSettingsAlerts();

  const steps = (): Step[] | null => {
    if (domains.isPending || sites.isPending) return null;
    const list: Step[] = [
      { label: "Add a domain", done: (domains.data ?? []).length > 0, to: "/domains" },
      {
        label: "Point its DNS at this server and verify it",
        done: (domains.data ?? []).some((d) => d.verified),
        to: "/domains",
      },
      { label: "Create a website", done: (sites.data ?? []).length > 0, to: "/websites" },
    ];
    if (props.isAdmin) {
      list.push({
        label: "Add an alert email",
        done: Boolean(alerts.data?.email.enabled),
        to: "/settings",
      });
    }
    return list;
  };

  const remaining = () => steps()?.filter((s) => !s.done).length ?? 0;

  return (
    <Show when={steps()}>
      {(list) => (
        <Show when={remaining() > 0}>
          <Card>
            <CardHeader>
              <CardTitle>Getting started</CardTitle>
              <CardDescription>
                {remaining()} {remaining() === 1 ? "step" : "steps"} left to get your first website
                live.
              </CardDescription>
            </CardHeader>
            <CardContent>
              <ol class="space-y-2 text-sm">
                <For each={list()}>
                  {(step, i) => (
                    <li class="flex items-center gap-3">
                      <Show
                        when={step.done}
                        fallback={<CircleDashed aria-hidden class="size-4 text-muted-foreground" />}
                      >
                        <Check aria-hidden class="size-4 text-emerald-600 dark:text-emerald-400" />
                      </Show>
                      <span
                        class={step.done ? "text-muted-foreground line-through" : "font-medium"}
                      >
                        {i() + 1}. {step.label}
                      </span>
                      <Show when={!step.done}>
                        <Link
                          to={step.to}
                          class="ml-auto inline-flex items-center gap-1 text-xs font-medium text-primary hover:underline"
                        >
                          Go <ArrowRight aria-hidden class="size-3" />
                        </Link>
                      </Show>
                    </li>
                  )}
                </For>
              </ol>
            </CardContent>
          </Card>
        </Show>
      )}
    </Show>
  );
}
