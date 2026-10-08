import { useMetrics } from "@/api/queries";
import { Alert } from "@/components/ui/badge-alert";
import { formatBytes, formatUptime, usedPercent } from "@/lib/format";

function Stat({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return (
    <div className="min-w-0">
      <p className="text-xs uppercase tracking-wide text-muted-foreground">{label}</p>
      <p className="truncate text-sm font-medium tabular-nums">{value}</p>
      {detail && <p className="text-xs text-muted-foreground tabular-nums">{detail}</p>}
    </div>
  );
}

/** Live CPU, memory, disk and uptime of this server. Updates every 5 seconds. */
export function ServerStrip() {
  const { data, isError } = useMetrics();

  if (isError) return <Alert tone="danger">Server status is unavailable right now.</Alert>;
  if (!data) return <p className="text-sm text-muted-foreground">Loading server status…</p>;

  const mem = usedPercent(data.mem_used_bytes, data.mem_total_bytes);
  const disk = usedPercent(data.disk_used_bytes, data.disk_total_bytes);

  return (
    <div className="grid grid-cols-2 gap-4 rounded-lg border border-border bg-card p-4 sm:grid-cols-4">
      <Stat
        label="Load"
        value={data.load_1m.toFixed(2)}
        detail={`${data.cpu_count} CPU${data.cpu_count === 1 ? "" : "s"}`}
      />
      <Stat
        label="Memory"
        value={`${mem ?? 0}% used`}
        detail={`${formatBytes(data.mem_used_bytes)} of ${formatBytes(data.mem_total_bytes)}`}
      />
      <Stat
        label="Disk"
        value={`${disk ?? 0}% used`}
        detail={`${formatBytes(data.disk_used_bytes)} of ${formatBytes(data.disk_total_bytes)}`}
      />
      <Stat label="Uptime" value={formatUptime(data.uptime_seconds)} />
    </div>
  );
}
