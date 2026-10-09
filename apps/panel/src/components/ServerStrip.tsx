import { For, Show, type JSX } from "solid-js";
import { useMetrics } from "@/api/queries";
import { Cpu, HardDrive, MemoryStick, Timer } from "@/components/icons";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { formatBytes, formatUptime, usedPercent } from "@/lib/format";
import { cn } from "@/lib/utils";

function Tile(props: {
  icon: JSX.Element;
  label: string;
  value: JSX.Element;
  detail: string;
  percent?: number | null;
}) {
  const barTone = () =>
    props.percent === undefined || props.percent === null
      ? ""
      : props.percent >= 90
        ? "bg-destructive"
        : props.percent >= 75
          ? "bg-warning"
          : "bg-primary";
  return (
    <div class="flex flex-col gap-2 rounded-lg border border-border bg-card p-4">
      <div class="flex items-center justify-between text-muted-foreground">
        <span class="text-xs font-medium uppercase tracking-wide">{props.label}</span>
        <span class="[&_svg]:size-4" aria-hidden="true">
          {props.icon}
        </span>
      </div>
      <div class="text-2xl font-semibold tabular-nums tracking-tight">{props.value}</div>
      <Show when={props.percent !== undefined}>
        <div class="h-1.5 overflow-hidden rounded-full bg-muted" role="presentation">
          <div
            class={cn("h-full rounded-full transition-[width] duration-500", barTone())}
            style={{ width: `${Math.min(100, props.percent ?? 0)}%` }}
          />
        </div>
      </Show>
      <p class="text-xs tabular-nums text-muted-foreground">{props.detail}</p>
    </div>
  );
}

/** Four headline numbers for this server. Updates every 5 seconds. */
export function ServerStrip() {
  const metrics = useMetrics();

  return (
    <Show
      when={!metrics.isError}
      fallback={
        <Alert variant="destructive">
          <AlertDescription>Server status is unavailable right now.</AlertDescription>
        </Alert>
      }
    >
      <Show
        when={metrics.data}
        fallback={
          <div class="grid grid-cols-2 gap-4 lg:grid-cols-4" aria-label="Loading server status">
            <For each={[0, 1, 2, 3]}>{() => <Skeleton class="h-28" />}</For>
          </div>
        }
      >
        {(data) => {
          const mem = () => usedPercent(data().mem_used_bytes, data().mem_total_bytes);
          const disk = () => usedPercent(data().disk_used_bytes, data().disk_total_bytes);
          const cpuPct = () => usedPercent(data().load_1m, data().cpu_count);
          return (
            <div class="grid grid-cols-2 gap-4 lg:grid-cols-4">
              <Tile
                icon={<Cpu />}
                label="CPU load"
                value={data().load_1m.toFixed(2)}
                detail={`${data().cpu_count} core${data().cpu_count === 1 ? "" : "s"} · 1 min average`}
                percent={cpuPct()}
              />
              <Tile
                icon={<MemoryStick />}
                label="Memory"
                value={`${mem() ?? 0}%`}
                detail={`${formatBytes(data().mem_used_bytes)} of ${formatBytes(data().mem_total_bytes)}`}
                percent={mem()}
              />
              <Tile
                icon={<HardDrive />}
                label="Disk"
                value={`${disk() ?? 0}%`}
                detail={`${formatBytes(data().disk_used_bytes)} of ${formatBytes(data().disk_total_bytes)}`}
                percent={disk()}
              />
              <Tile
                icon={<Timer />}
                label="Uptime"
                value={formatUptime(data().uptime_seconds)}
                detail="since last reboot"
              />
            </div>
          );
        }}
      </Show>
    </Show>
  );
}
