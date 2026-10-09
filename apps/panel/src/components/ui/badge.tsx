import { splitProps, type JSX } from "solid-js";
import { cn } from "@/lib/utils";

export type BadgeVariant = "default" | "outline" | "secondary";

const VARIANTS: Record<BadgeVariant, string> = {
  default: "border-transparent bg-primary text-primary-foreground",
  outline: "border-border",
  secondary: "border-transparent bg-muted text-foreground",
};

export function Badge(props: JSX.HTMLAttributes<HTMLSpanElement> & { variant?: BadgeVariant }) {
  const [own, rest] = splitProps(props, ["variant", "class", "children"]);
  return (
    <span
      class={cn(
        "inline-flex items-center gap-1 rounded-md border px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        VARIANTS[own.variant ?? "default"],
        own.class,
      )}
      {...rest}
    >
      {own.children}
    </span>
  );
}
