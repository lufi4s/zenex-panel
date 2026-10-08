import type { HTMLAttributes } from "react";
import { cn } from "@/lib/utils";

const VARIANTS = {
  default: "border-border bg-card text-foreground",
  destructive: "border-destructive/40 bg-destructive/5 text-destructive",
} as const;

export function Alert({
  variant = "default",
  className,
  ...props
}: HTMLAttributes<HTMLDivElement> & { variant?: keyof typeof VARIANTS }) {
  return (
    <div
      role={variant === "destructive" ? "alert" : undefined}
      className={cn(
        "relative w-full rounded-lg border px-4 py-3 text-sm",
        VARIANTS[variant],
        className,
      )}
      {...props}
    />
  );
}

export function AlertDescription({ className, ...props }: HTMLAttributes<HTMLDivElement>) {
  return <div className={cn("text-sm leading-relaxed", className)} {...props} />;
}
