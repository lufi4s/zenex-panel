import { For, Show } from "solid-js";
import { Layers } from "@/components/icons";
import { Link } from "@/lib/router";
import { useMe, useSites, useSystemUpdate } from "@/api/queries";
import type { Site } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { buttonClasses } from "@/components/ui/button";
import {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { CardSkeleton } from "@/components/CardSkeleton";
import { PageHeader } from "@/components/PageHeader";
import { ServerStrip } from "@/components/ServerStrip";
import { ServicesCard } from "@/components/ServicesCard";
import { badgeTone } from "@/lib/badge";
import { STATE_LABEL, STATE_TONE } from "@/lib/site-status";

function RecentSites(props: { sites: Site[] }) {
  return (
    <Show
      when={props.sites.length > 0}
      fallback={
        <p class="py-6 text-center text-sm text-muted-foreground">
          No websites yet. Create your first one from Websites.
        </p>
      }
    >
      <ul class="divide-y divide-border">
        <For each={props.sites.slice(0, 5)}>
          {(site) => (
            <li>
              <Link
                to={`/websites/${site.id}`}
                class="flex items-center justify-between gap-3 rounded-md py-2.5 hover:bg-muted/50"
              >
                <span class="min-w-0 truncate font-medium">{site.domain}</span>
                <Badge variant="outline" class={badgeTone(STATE_TONE[site.state])}>
                  {STATE_LABEL[site.state]}
                </Badge>
              </Link>
            </li>
          )}
        </For>
      </ul>
    </Show>
  );
}

/** Landing page: server health, a quick look at the websites, and service status. */
export function OverviewPage() {
  const sites = useSites();
  const me = useMe();
  const isAdmin = me.data?.roles.includes("administrator") ?? false;
  const update = useSystemUpdate(isAdmin);
  const updateReady = () => update.data?.update_available && update.data.state !== "running";
  const list = () => sites.data ?? [];
  const live = () => list().filter((s) => s.state === "ready").length;
  const building = () => list().filter((s) => s.state === "provisioning").length;

  return (
    <>
      <PageHeader
        title="Overview"
        description="How your server and websites are doing right now."
      />
      <Show when={updateReady()}>
        <Alert>
          <AlertDescription class="flex flex-wrap items-center justify-between gap-3">
            <span>A new version of the panel is available ({update.data?.latest}).</span>
            <Link to="/settings" class={buttonClasses("default", "sm")}>
              Update in Settings
            </Link>
          </AlertDescription>
        </Alert>
      </Show>
      <ServerStrip />

      <div class="grid gap-4 lg:grid-cols-5">
        <Card class="lg:col-span-3">
          <CardHeader>
            <CardTitle>Websites</CardTitle>
            <CardDescription>
              {sites.isPending
                ? "Loading…"
                : `${live()} live${building() ? ` · ${building()} building` : ""} · ${list().length} total`}
            </CardDescription>
            <CardAction>
              <Link to="/websites" class={buttonClasses("outline", "sm")}>
                <Layers aria-hidden /> All websites
              </Link>
            </CardAction>
          </CardHeader>
          <CardContent>
            <Show when={!sites.isPending} fallback={<CardSkeleton height="h-40" />}>
              <RecentSites sites={list()} />
            </Show>
          </CardContent>
        </Card>

        <div class="lg:col-span-2">
          <ServicesCard />
        </div>
      </div>
    </>
  );
}
