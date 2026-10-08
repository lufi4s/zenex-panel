import { Layers } from "@/components/icons";
import { Link } from "@/lib/router";
import { useMe, useSites, useSystemUpdate } from "@/api/queries";
import type { Site } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
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

function RecentSites({ sites }: { sites: Site[] }) {
  if (sites.length === 0) {
    return (
      <p className="py-6 text-center text-sm text-muted-foreground">
        No websites yet. Create your first one from Websites.
      </p>
    );
  }
  return (
    <ul className="divide-y divide-border">
      {sites.slice(0, 5).map((site) => (
        <li key={site.id}>
          <Link
            to={`/websites/${site.id}`}
            className="flex items-center justify-between gap-3 rounded-md py-2.5 hover:bg-muted/50"
          >
            <span className="min-w-0 truncate font-medium">{site.domain}</span>
            <Badge variant="outline" className={badgeTone(STATE_TONE[site.state])}>
              {STATE_LABEL[site.state]}
            </Badge>
          </Link>
        </li>
      ))}
    </ul>
  );
}

/** Landing page: server health, a quick look at the websites, and service status. */
export function OverviewPage() {
  const sites = useSites();
  const me = useMe();
  const isAdmin = me.data?.roles.includes("administrator") ?? false;
  const update = useSystemUpdate(isAdmin);
  const updateReady = update.data?.update_available && update.data.state !== "running";
  const list = sites.data ?? [];
  const live = list.filter((s) => s.state === "ready").length;
  const building = list.filter((s) => s.state === "provisioning").length;

  return (
    <>
      <PageHeader
        title="Overview"
        description="How your server and websites are doing right now."
      />
      {updateReady && (
        <Alert>
          <AlertDescription className="flex flex-wrap items-center justify-between gap-3">
            <span>A new version of the panel is available ({update.data?.latest}).</span>
            <Button size="sm" asChild>
              <Link to="/settings">Update in Settings</Link>
            </Button>
          </AlertDescription>
        </Alert>
      )}
      <ServerStrip />

      <div className="grid gap-4 lg:grid-cols-5">
        <Card className="lg:col-span-3">
          <CardHeader>
            <CardTitle>Websites</CardTitle>
            <CardDescription>
              {sites.isPending
                ? "Loading…"
                : `${live} live${building ? ` · ${building} building` : ""} · ${list.length} total`}
            </CardDescription>
            <CardAction>
              <Button variant="outline" size="sm" asChild>
                <Link to="/websites">
                  <Layers aria-hidden /> All websites
                </Link>
              </Button>
            </CardAction>
          </CardHeader>
          <CardContent>
            {sites.isPending ? <CardSkeleton height="h-40" /> : <RecentSites sites={list} />}
          </CardContent>
        </Card>

        <div className="lg:col-span-2">
          <ServicesCard />
        </div>
      </div>
    </>
  );
}
