import { splitProps, type JSX } from "solid-js";
import { cn } from "@/lib/utils";

export function Skeleton(props: JSX.HTMLAttributes<HTMLDivElement>) {
  const [own, rest] = splitProps(props, ["class"]);
  return <div class={cn("animate-pulse rounded-md bg-muted", own.class)} {...rest} />;
}
