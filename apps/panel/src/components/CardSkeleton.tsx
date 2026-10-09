import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";

/** A card-shaped placeholder for lazily loaded sections. */
export function CardSkeleton(props: { height?: string }) {
  return (
    <div
      role="status"
      aria-label="Loading section"
      class="rounded-xl border border-border bg-card p-6"
    >
      <Skeleton class="mb-4 h-4 w-40" />
      <Skeleton class={cn("w-full", props.height ?? "h-48")} />
    </div>
  );
}
