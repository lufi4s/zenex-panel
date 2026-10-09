import { splitProps, type JSX } from "solid-js";
import { cn } from "@/lib/utils";

export function Label(props: JSX.LabelHTMLAttributes<HTMLLabelElement>) {
  const [own, rest] = splitProps(props, ["class", "children"]);
  return (
    <label class={cn("text-sm font-medium leading-none", own.class)} {...rest}>
      {own.children}
    </label>
  );
}
