export type StatusTone = "success" | "warning" | "danger" | "neutral";

/** Status colour classes for a shadcn badge (the badge itself is the official component). */
export function badgeTone(tone: StatusTone | string): string {
  switch (tone) {
    case "success":
      return "border-success/40 bg-success/10 text-success";
    case "warning":
      return "border-warning/40 bg-warning/10 text-warning";
    case "danger":
      return "border-destructive/40 bg-destructive/10 text-destructive";
    default:
      return "";
  }
}
