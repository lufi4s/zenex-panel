const UNITS = ["B", "KB", "MB", "GB", "TB"] as const;

/** 1536 -> "1.5 KB". Binary units, one decimal place above bytes. */
export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return "—";
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < UNITS.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toFixed(unit === 0 ? 0 : 1)} ${UNITS[unit]}`;
}

/** 90061 -> "1d 1h". Shows the two largest units only. */
export function formatUptime(seconds: number): string {
  if (!Number.isFinite(seconds) || seconds < 0) return "—";
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  if (days > 0) return `${days}d ${hours}h`;
  return `${hours}h ${minutes}m`;
}

/** Percentage of used memory or disk, or null when the total is unknown. */
export function usedPercent(used: number, total: number): number | null {
  if (total <= 0) return null;
  return Math.round((used / total) * 100);
}

/** The label a build step should show, e.g. "create_wp_config" -> "create wp config". */
export function stepLabel(name: string): string {
  return name.replace(/_/g, " ");
}

/** The address of a website, as a clickable link target. */
export function siteUrl(domain: string): string {
  return `http://${domain}`;
}
