import { cn } from "@/lib/utils";

/** A placeholder block shown while a section loads. */
export function Skeleton({ className }: { className?: string }) {
  return <div aria-hidden className={cn("animate-pulse rounded-md bg-muted", className)} />;
}

/** A card-shaped placeholder for lazily loaded sections. */
export function CardSkeleton({ height = "h-48" }: { height?: string }) {
  return (
    <div
      role="status"
      aria-label="Loading section"
      className="rounded-lg border border-border bg-card p-5"
    >
      <Skeleton className="mb-4 h-4 w-40" />
      <Skeleton className={cn("w-full", height)} />
    </div>
  );
}
