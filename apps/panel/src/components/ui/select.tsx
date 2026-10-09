import { splitProps, type JSX } from "solid-js";
import { ChevronDown } from "@/components/icons";
import { cn } from "@/lib/utils";

/**
 * A native select (keyboard, screen readers and mobile pickers keep working) with one
 * consistent look: the same height and border as the inputs, and a chevron on the right.
 */
export function Select(props: JSX.SelectHTMLAttributes<HTMLSelectElement>) {
  const [own, rest] = splitProps(props, ["class"]);
  return (
    <div class="relative">
      <select
        {...rest}
        class={cn(
          "flex h-10 w-full appearance-none rounded-md border border-input bg-card py-0 pl-3 pr-10 text-base text-foreground sm:h-9 sm:text-sm",
          "focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/30",
          "disabled:cursor-not-allowed disabled:opacity-50",
          own.class,
        )}
      />
      <ChevronDown
        aria-hidden="true"
        class="pointer-events-none absolute right-3 top-1/2 size-4 -translate-y-1/2 text-muted-foreground"
      />
    </div>
  );
}
