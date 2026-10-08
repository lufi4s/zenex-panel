import { useMemo, useState } from "react";
import { useMetricSeries } from "@/api/queries";
import type { TimeRange } from "@/api/types";
import { Alert } from "@/components/ui/badge-alert";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { LineChart } from "@/components/LineChart";
import { cn } from "@/lib/utils";

const RANGES: { value: TimeRange; label: string }[] = [
  { value: "1h", label: "1 hour" },
  { value: "6h", label: "6 hours" },
  { value: "24h", label: "24 hours" },
  { value: "7d", label: "7 days" },
];

const COLORS = {
  load: "oklch(0.62 0.17 262)",
  memory: "oklch(0.66 0.16 150)",
  disk: "oklch(0.7 0.15 70)",
};

function percent(v: number): string {
  return `${v.toFixed(0)}%`;
}

function load(v: number): string {
  return v.toFixed(2);
}

/** Summary of one metric across the selected range: latest, peak and average. */
function Summary({
  label,
  latest,
  peak,
  average,
  color,
}: {
  label: string;
  latest: string;
  peak: string;
  average: string;
  color: string;
}) {
  return (
    <div className="min-w-0 rounded-md border border-border p-3">
      <div className="flex items-center gap-2 text-xs uppercase tracking-wide text-muted-foreground">
        <span
          className="inline-block size-2 rounded-full"
          style={{ background: color }}
          aria-hidden
        />
        {label}
      </div>
      <div className="mt-1 text-lg font-semibold tabular-nums">{latest}</div>
      <div className="text-xs text-muted-foreground tabular-nums">
        peak {peak} · avg {average}
      </div>
    </div>
  );
}

/** Host history with range selection, hover details and per-metric summaries. */
export function MonitoringCard() {
  const [range, setRange] = useState<TimeRange>("1h");
  const series = useMetricSeries(range);
  const points = series.data?.points ?? [];

  const model = useMemo(() => {
    const times = points.map((p) => new Date(p.t).getTime());
    const stats = (values: number[]) => {
      if (values.length === 0) return { latest: 0, peak: 0, average: 0 };
      const sum = values.reduce((a, b) => a + b, 0);
      return {
        latest: values[values.length - 1],
        peak: Math.max(...values),
        average: sum / values.length,
      };
    };
    const loadValues = points.map((p) => p.load);
    const memValues = points.map((p) => p.mem_pct);
    const diskValues = points.map((p) => p.disk_pct);
    return {
      times,
      load: stats(loadValues),
      mem: stats(memValues),
      disk: stats(diskValues),
      loadValues,
      memValues,
      diskValues,
    };
  }, [points]);

  return (
    <Card>
      <CardHeader className="flex-row flex-wrap items-center justify-between gap-3">
        <div>
          <CardTitle>Server history</CardTitle>
          <CardDescription>CPU load, memory and disk, sampled every 30 seconds.</CardDescription>
        </div>
        <div
          role="group"
          aria-label="Time range"
          className="inline-flex rounded-md border border-border p-0.5"
        >
          {RANGES.map((r) => (
            <button
              key={r.value}
              type="button"
              aria-pressed={range === r.value}
              onClick={() => setRange(r.value)}
              className={cn(
                "rounded-sm px-2.5 py-1 text-xs font-medium transition-colors",
                range === r.value
                  ? "bg-primary text-primary-foreground"
                  : "text-muted-foreground hover:text-foreground",
              )}
            >
              {r.label}
            </button>
          ))}
        </div>
      </CardHeader>
      <CardContent className="space-y-4">
        {series.isError && <Alert tone="danger">Could not load history right now.</Alert>}

        <div className="grid grid-cols-1 gap-3 sm:grid-cols-3">
          <Summary
            label="Load"
            latest={load(model.load.latest)}
            peak={load(model.load.peak)}
            average={load(model.load.average)}
            color={COLORS.load}
          />
          <Summary
            label="Memory"
            latest={percent(model.mem.latest)}
            peak={percent(model.mem.peak)}
            average={percent(model.mem.average)}
            color={COLORS.memory}
          />
          <Summary
            label="Disk"
            latest={percent(model.disk.latest)}
            peak={percent(model.disk.peak)}
            average={percent(model.disk.average)}
            color={COLORS.disk}
          />
        </div>

        <LineChart
          title="CPU load"
          times={model.times}
          series={[{ label: "Load", color: COLORS.load, values: model.loadValues }]}
          format={load}
        />
        <LineChart
          title="Memory use"
          times={model.times}
          domain={[0, 100]}
          series={[{ label: "Memory", color: COLORS.memory, values: model.memValues }]}
          format={percent}
        />
        <LineChart
          title="Disk use"
          times={model.times}
          domain={[0, 100]}
          series={[{ label: "Disk", color: COLORS.disk, values: model.diskValues }]}
          format={percent}
        />
      </CardContent>
    </Card>
  );
}
