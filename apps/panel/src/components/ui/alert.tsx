import { splitProps, type JSX } from "solid-js";
import { cn } from "@/lib/utils";

const VARIANTS = {
  default: "border-border bg-card text-foreground",
  destructive: "border-destructive/40 bg-destructive/5 text-destructive",
} as const;

export function Alert(
  props: JSX.HTMLAttributes<HTMLDivElement> & { variant?: keyof typeof VARIANTS },
) {
  const [own, rest] = splitProps(props, ["variant", "class", "children"]);
  const variant = own.variant ?? "default";
  return (
    <div
      role={variant === "destructive" ? "alert" : undefined}
      class={cn(
        "relative w-full rounded-lg border px-4 py-3 text-sm",
        VARIANTS[variant],
        own.class,
      )}
      {...rest}
    >
      {own.children}
    </div>
  );
}

export function AlertDescription(props: JSX.HTMLAttributes<HTMLDivElement>) {
  const [own, rest] = splitProps(props, ["class", "children"]);
  return (
    <div class={cn("text-sm leading-relaxed", own.class)} {...rest}>
      {own.children}
    </div>
  );
}
