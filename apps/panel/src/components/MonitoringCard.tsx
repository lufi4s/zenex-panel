import { createMemo, createSignal, For, Show } from "solid-js";
import { useMetricSeries } from "@/api/queries";
import type { TimeRange } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
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
function Summary(props: {
  label: string;
  latest: string;
  peak: string;
  average: string;
  color: string;
}) {
  return (
    <div class="min-w-0 rounded-md border border-border p-3">
      <div class="flex items-center gap-2 text-xs uppercase tracking-wide text-muted-foreground">
        <span
          class="inline-block size-2 rounded-full"
          style={{ background: props.color }}
          aria-hidden="true"
        />
        {props.label}
      </div>
      <div class="mt-1 text-lg font-semibold tabular-nums">{props.latest}</div>
      <div class="text-xs text-muted-foreground tabular-nums">
        peak {props.peak} · avg {props.average}
      </div>
    </div>
  );
}

/**
 * The data and charts for one range. A new range mounts a new query, because the
 * metric query reads its range once when it starts.
 */
function MonitoringBody(props: { range: TimeRange }) {
  const series = useMetricSeries(props.range);
  const points = () => series.data?.points ?? [];

  const model = createMemo(() => {
    const pts = points();
    const times = pts.map((p) => new Date(p.t).getTime());
    const stats = (values: number[]) => {
      if (values.length === 0) return { latest: 0, peak: 0, average: 0 };
      const sum = values.reduce((a, b) => a + b, 0);
      return {
        latest: values[values.length - 1],
        peak: Math.max(...values),
        average: sum / values.length,
      };
    };
    const loadValues = pts.map((p) => p.load);
    const memValues = pts.map((p) => p.mem_pct);
    const diskValues = pts.map((p) => p.disk_pct);
    return {
      times,
      load: stats(loadValues),
      mem: stats(memValues),
      disk: stats(diskValues),
      loadValues,
      memValues,
      diskValues,
    };
  });

  return (
    <>
      <Show when={series.isError}>
        <Alert variant="destructive">
          <AlertDescription>Could not load history right now.</AlertDescription>
        </Alert>
      </Show>

      <div class="grid grid-cols-1 gap-3 sm:grid-cols-3">
        <Summary
          label="Load"
          latest={load(model().load.latest)}
          peak={load(model().load.peak)}
          average={load(model().load.average)}
          color={COLORS.load}
        />
        <Summary
          label="Memory"
          latest={percent(model().mem.latest)}
          peak={percent(model().mem.peak)}
          average={percent(model().mem.average)}
          color={COLORS.memory}
        />
        <Summary
          label="Disk"
          latest={percent(model().disk.latest)}
          peak={percent(model().disk.peak)}
          average={percent(model().disk.average)}
          color={COLORS.disk}
        />
      </div>

      <LineChart
        title="CPU load"
        times={model().times}
        series={[{ label: "Load", color: COLORS.load, values: model().loadValues }]}
        format={load}
      />
      <LineChart
        title="Memory use"
        times={model().times}
        domain={[0, 100]}
        series={[{ label: "Memory", color: COLORS.memory, values: model().memValues }]}
        format={percent}
      />
      <LineChart
        title="Disk use"
        times={model().times}
        domain={[0, 100]}
        series={[{ label: "Disk", color: COLORS.disk, values: model().diskValues }]}
        format={percent}
      />
    </>
  );
}

/** Host history with range selection, hover details and per-metric summaries. */
export function MonitoringCard() {
  const [range, setRange] = createSignal<TimeRange>("1h");

  return (
    <Card>
      <CardHeader class="flex-row flex-wrap items-center justify-between gap-3">
        <div>
          <CardTitle>Server history</CardTitle>
          <CardDescription>CPU load, memory and disk, sampled every 30 seconds.</CardDescription>
        </div>
        <div
          role="group"
          aria-label="Time range"
          class="inline-flex rounded-md border border-border p-0.5"
        >
          <For each={RANGES}>
            {(r) => (
              <button
                type="button"
                aria-pressed={range() === r.value}
                onClick={() => setRange(r.value)}
                class={cn(
                  "rounded-sm px-2.5 py-1 text-xs font-medium transition-colors",
                  range() === r.value
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                {r.label}
              </button>
            )}
          </For>
        </div>
      </CardHeader>
      <CardContent class="space-y-4">
        <Show when={range()} keyed>
          {(r) => <MonitoringBody range={r} />}
        </Show>
      </CardContent>
    </Card>
  );
}
