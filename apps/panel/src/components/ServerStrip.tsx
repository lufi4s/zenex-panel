import type { ReactNode } from "react";
import { Cpu, HardDrive, MemoryStick, Timer } from "lucide-react";
import { useMetrics } from "@/api/queries";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Skeleton } from "@/components/ui/skeleton";
import { formatBytes, formatUptime, usedPercent } from "@/lib/format";
import { cn } from "@/lib/utils";

function Tile({
  icon,
  label,
  value,
  detail,
  percent,
}: {
  icon: ReactNode;
  label: string;
  value: string;
  detail: string;
  percent?: number | null;
}) {
  const barTone =
    percent === undefined || percent === null
      ? ""
      : percent >= 90
        ? "bg-destructive"
        : percent >= 75
          ? "bg-warning"
          : "bg-primary";
  return (
    <div className="flex flex-col gap-2 rounded-lg border border-border bg-card p-4">
      <div className="flex items-center justify-between text-muted-foreground">
        <span className="text-xs font-medium uppercase tracking-wide">{label}</span>
        <span className="[&_svg]:size-4" aria-hidden>
          {icon}
        </span>
      </div>
      <div className="text-2xl font-semibold tabular-nums tracking-tight">{value}</div>
      {percent !== undefined && (
        <div className="h-1.5 overflow-hidden rounded-full bg-muted" role="presentation">
          <div
            className={cn("h-full rounded-full transition-[width] duration-500", barTone)}
            style={{ width: `${Math.min(100, percent ?? 0)}%` }}
          />
        </div>
      )}
      <p className="text-xs tabular-nums text-muted-foreground">{detail}</p>
    </div>
  );
}

/** Four headline numbers for this server. Updates every 5 seconds. */
export function ServerStrip() {
  const { data, isError, isPending } = useMetrics();

  if (isError)
    return (
      <Alert variant="destructive">
        <AlertDescription>Server status is unavailable right now.</AlertDescription>
      </Alert>
    );
  if (isPending || !data) {
    return (
      <div className="grid grid-cols-2 gap-4 lg:grid-cols-4" aria-label="Loading server status">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-28" />
        ))}
      </div>
    );
  }

  const mem = usedPercent(data.mem_used_bytes, data.mem_total_bytes);
  const disk = usedPercent(data.disk_used_bytes, data.disk_total_bytes);
  const cpuPct = usedPercent(data.load_1m, data.cpu_count);

  return (
    <div className="grid grid-cols-2 gap-4 lg:grid-cols-4">
      <Tile
        icon={<Cpu />}
        label="CPU load"
        value={data.load_1m.toFixed(2)}
        detail={`${data.cpu_count} core${data.cpu_count === 1 ? "" : "s"} · 1 min average`}
        percent={cpuPct}
      />
      <Tile
        icon={<MemoryStick />}
        label="Memory"
        value={`${mem ?? 0}%`}
        detail={`${formatBytes(data.mem_used_bytes)} of ${formatBytes(data.mem_total_bytes)}`}
        percent={mem}
      />
      <Tile
        icon={<HardDrive />}
        label="Disk"
        value={`${disk ?? 0}%`}
        detail={`${formatBytes(data.disk_used_bytes)} of ${formatBytes(data.disk_total_bytes)}`}
        percent={disk}
      />
      <Tile
        icon={<Timer />}
        label="Uptime"
        value={formatUptime(data.uptime_seconds)}
        detail="since last reboot"
      />
    </div>
  );
}
