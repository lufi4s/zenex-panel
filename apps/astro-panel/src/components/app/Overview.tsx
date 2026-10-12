import type { LucideIcon } from "lucide-react";
import { Area, AreaChart, CartesianGrid, XAxis, YAxis } from "recharts";
import { Clock, Cpu, HardDrive, MemoryStick } from "lucide-react";
import { useMetrics } from "@/lib/queries";
import { formatBytes, formatUptime, percent } from "@/components/app/format";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { ChartContainer, type ChartConfig } from "@/components/ui/chart";
import { Progress } from "@/components/ui/progress";
import { Skeleton } from "@/components/ui/skeleton";

const chartConfig = {
  load: { label: "Load average", color: "var(--primary)" },
} satisfies ChartConfig;

/** Server health at a glance. Refreshes every 15 seconds while the tab is open. */
export function Overview() {
  const { data: m, isPending, isError } = useMetrics();

  if (isPending) {
    return (
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        {Array.from({ length: 4 }, (_, i) => (
          <Skeleton key={i} className="h-32 rounded-xl" />
        ))}
      </div>
    );
  }
  if (isError || !m) {
    return <p className="text-sm text-destructive">Server health could not be loaded.</p>;
  }

  const cpuPct = Math.min(100, percent(m.load_1m, m.cpu_count));
  const memPct = percent(m.mem_used_bytes, m.mem_total_bytes);
  const diskPct = percent(m.disk_used_bytes, m.disk_total_bytes);
  const loads = [
    { name: "1 min", load: m.load_1m },
    { name: "5 min", load: m.load_5m },
    { name: "15 min", load: m.load_15m },
  ];

  return (
    <div className="space-y-6">
      <div className="grid gap-4 sm:grid-cols-2 xl:grid-cols-4">
        <MetricCard
          icon={Cpu}
          title="CPU"
          value={`${cpuPct}%`}
          detail={`${m.cpu_count} cores, load ${m.load_1m.toFixed(2)}`}
          pct={cpuPct}
        />
        <MetricCard
          icon={MemoryStick}
          title="Memory"
          value={`${memPct}%`}
          detail={`${formatBytes(m.mem_used_bytes)} of ${formatBytes(m.mem_total_bytes)}`}
          pct={memPct}
        />
        <MetricCard
          icon={HardDrive}
          title="Disk"
          value={`${diskPct}%`}
          detail={`${formatBytes(m.disk_used_bytes)} of ${formatBytes(m.disk_total_bytes)}`}
          pct={diskPct}
        />
        <MetricCard icon={Clock} title="Uptime" value={formatUptime(m.uptime_seconds)} detail="Since the last restart" />
      </div>

      <Card>
        <CardHeader>
          <CardTitle>Server load</CardTitle>
          <CardDescription>Load average over 1, 5 and 15 minutes</CardDescription>
        </CardHeader>
        <CardContent>
          <ChartContainer config={chartConfig} className="h-56 w-full">
            <AreaChart data={loads}>
              <CartesianGrid vertical={false} strokeDasharray="3 3" />
              <XAxis dataKey="name" tickLine={false} axisLine={false} />
              <YAxis tickLine={false} axisLine={false} width={32} />
              <Area
                dataKey="load"
                type="monotone"
                stroke="var(--color-load)"
                fill="var(--color-load)"
                fillOpacity={0.15}
                strokeWidth={2}
              />
            </AreaChart>
          </ChartContainer>
        </CardContent>
      </Card>
    </div>
  );
}

function MetricCard({
  icon: Icon,
  title,
  value,
  detail,
  pct,
}: {
  icon: LucideIcon;
  title: string;
  value: string;
  detail: string;
  pct?: number;
}) {
  return (
    <Card className="gap-3 transition-colors hover:border-primary/40">
      <CardHeader className="flex flex-row items-center justify-between">
        <CardDescription>{title}</CardDescription>
        <Icon className="size-4 text-muted-foreground" />
      </CardHeader>
      <CardContent className="space-y-3">
        <p className="text-2xl font-semibold tabular-nums">{value}</p>
        {pct !== undefined && <Progress value={pct} />}
        <p className="text-xs text-muted-foreground">{detail}</p>
      </CardContent>
    </Card>
  );
}
