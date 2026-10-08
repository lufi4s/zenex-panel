import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

/** A card-shaped placeholder for lazily loaded sections. */
export function CardSkeleton({ height = "h-48" }: { height?: string }) {
  return (
    <div
      role="status"
      aria-label="Loading section"
      className="rounded-xl border border-border bg-card p-6"
    >
      <Skeleton className="mb-4 h-4 w-40" />
      <Skeleton className={cn("w-full", height)} />
    </div>
  );
}
