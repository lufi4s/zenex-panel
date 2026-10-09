import { Show } from "solid-js";
import { useMetrics } from "@/api/queries";
import { MonitoringCard } from "@/components/MonitoringCard";
import { PageHeader } from "@/components/PageHeader";
import { ServerStrip } from "@/components/ServerStrip";
import { ServicesCard } from "@/components/ServicesCard";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { formatBytes, formatUptime } from "@/lib/format";

/** The load averages and capacity numbers behind the headline tiles. */
function DetailsCard() {
  const metrics = useMetrics();
  return (
    <Card>
      <CardHeader>
        <CardTitle>Capacity and load</CardTitle>
        <CardDescription>
          Averages and totals for this server, refreshed every 5 seconds.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Show when={metrics.data}>
          {(m) => (
            <dl class="grid grid-cols-2 gap-x-6 gap-y-4 text-sm sm:grid-cols-4">
              <div>
                <dt class="text-muted-foreground">Load 1 min</dt>
                <dd class="mt-1 font-semibold tabular-nums">{m().load_1m.toFixed(2)}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Load 5 min</dt>
                <dd class="mt-1 font-semibold tabular-nums">{m().load_5m.toFixed(2)}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Load 15 min</dt>
                <dd class="mt-1 font-semibold tabular-nums">{m().load_15m.toFixed(2)}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">CPU cores</dt>
                <dd class="mt-1 font-semibold tabular-nums">{m().cpu_count}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Memory total</dt>
                <dd class="mt-1 font-semibold tabular-nums">{formatBytes(m().mem_total_bytes)}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Disk total</dt>
                <dd class="mt-1 font-semibold tabular-nums">{formatBytes(m().disk_total_bytes)}</dd>
              </div>
              <div>
                <dt class="text-muted-foreground">Uptime</dt>
                <dd class="mt-1 font-semibold">{formatUptime(m().uptime_seconds)}</dd>
              </div>
            </dl>
          )}
        </Show>
      </CardContent>
    </Card>
  );
}

/** Server health: headline numbers, capacity and load, resource history, and service status. */
export function ServerPage() {
  return (
    <>
      <PageHeader
        title="Server"
        description="CPU, memory, disk and network over time, plus the services that run this panel."
      />
      <ServerStrip />
      <DetailsCard />
      <MonitoringCard />
      <ServicesCard />
    </>
  );
}
