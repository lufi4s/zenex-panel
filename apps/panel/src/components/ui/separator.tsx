import { splitProps, type JSX } from "solid-js";
import { cn } from "@/lib/utils";

export function Separator(
  props: JSX.HTMLAttributes<HTMLDivElement> & { orientation?: "horizontal" | "vertical" },
) {
  const [own, rest] = splitProps(props, ["orientation", "class"]);
  const orientation = own.orientation ?? "horizontal";
  return (
    <div
      role="separator"
      aria-orientation={orientation}
      class={cn(
        "shrink-0 bg-border",
        orientation === "vertical" ? "w-px self-stretch" : "h-px w-full",
        own.class,
      )}
      {...rest}
    />
  );
}
