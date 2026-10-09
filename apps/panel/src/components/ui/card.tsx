import { splitProps, type JSX } from "solid-js";
import { cn } from "@/lib/utils";

type DivProps = JSX.HTMLAttributes<HTMLDivElement>;

export function Card(props: DivProps) {
  const [own, rest] = splitProps(props, ["class", "children"]);
  return (
    <div
      class={cn(
        "rounded-xl border border-border bg-card text-card-foreground shadow-sm",
        own.class,
      )}
      {...rest}
    >
      {own.children}
    </div>
  );
}

export function CardHeader(props: DivProps) {
  const [own, rest] = splitProps(props, ["class", "children"]);
  return (
    <div class={cn("flex flex-col gap-1.5 p-5 pb-3", own.class)} {...rest}>
      {own.children}
    </div>
  );
}

export function CardTitle(props: DivProps) {
  const [own, rest] = splitProps(props, ["class", "children"]);
  return (
    <div
      class={cn("font-heading text-base font-semibold leading-none tracking-tight", own.class)}
      {...rest}
    >
      {own.children}
    </div>
  );
}

export function CardDescription(props: DivProps) {
  const [own, rest] = splitProps(props, ["class", "children"]);
  return (
    <div class={cn("text-sm text-muted-foreground", own.class)} {...rest}>
      {own.children}
    </div>
  );
}

/** Optional action area on the right of a card header. */
export function CardAction(props: DivProps) {
  const [own, rest] = splitProps(props, ["class", "children"]);
  return (
    <div class={cn("flex items-center gap-2", own.class)} {...rest}>
      {own.children}
    </div>
  );
}

export function CardContent(props: DivProps) {
  const [own, rest] = splitProps(props, ["class", "children"]);
  return (
    <div class={cn("p-5 pt-2", own.class)} {...rest}>
      {own.children}
    </div>
  );
}

export function CardFooter(props: DivProps) {
  const [own, rest] = splitProps(props, ["class", "children"]);
  return (
    <div class={cn("flex items-center p-5 pt-0", own.class)} {...rest}>
      {own.children}
    </div>
  );
}
