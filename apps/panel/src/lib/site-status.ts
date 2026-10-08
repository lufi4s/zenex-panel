import type { SiteState } from "@/api/types";
import type { StatusTone } from "@/lib/badge";

export const STATE_LABEL: Record<SiteState, string> = {
  ready: "Live",
  provisioning: "Building",
  suspended: "Offline",
  failed: "Failed",
  deleting: "Deleting",
  deleted: "Deleted",
};

export const STATE_TONE: Record<SiteState, StatusTone> = {
  ready: "success",
  provisioning: "warning",
  suspended: "neutral",
  failed: "danger",
  deleting: "warning",
  deleted: "neutral",
};

/** Short "2 min ago" style text for a timestamp. */
export function timeAgo(iso: string): string {
  const seconds = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 1000));
  if (seconds < 60) return "just now";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} min ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 24) return `${hours} h ago`;
  return new Date(iso).toLocaleDateString(undefined, {
    month: "short",
    day: "numeric",
    year: "numeric",
  });
}
