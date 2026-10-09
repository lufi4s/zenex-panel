import { splitProps, type JSX } from "solid-js";
import { cn } from "@/lib/utils";

export function Input(props: JSX.InputHTMLAttributes<HTMLInputElement>) {
  const [own, rest] = splitProps(props, ["class"]);
  return (
    <input
      class={cn(
        "flex h-10 w-full rounded-md border border-input bg-background px-3 text-base sm:h-9 sm:text-sm",
        "placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/30",
        "disabled:cursor-not-allowed disabled:opacity-50",
        own.class,
      )}
      {...rest}
    />
  );
}
