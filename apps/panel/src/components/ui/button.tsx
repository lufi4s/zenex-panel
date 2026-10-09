import { splitProps, type JSX } from "solid-js";
import { cn } from "@/lib/utils";

export type ButtonVariant = "default" | "outline" | "ghost" | "destructive" | "secondary" | "link";
export type ButtonSize = "default" | "sm" | "lg" | "icon";

const BASE =
  "inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-md text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40 disabled:pointer-events-none disabled:opacity-50 [&_svg]:size-4 [&_svg]:shrink-0";

const VARIANTS: Record<ButtonVariant, string> = {
  default: "bg-primary text-primary-foreground hover:bg-primary/90",
  outline: "border border-border bg-background hover:bg-muted",
  ghost: "hover:bg-muted",
  destructive: "bg-destructive text-white hover:bg-destructive/90",
  secondary: "bg-muted text-foreground hover:bg-muted/80",
  link: "text-primary underline-offset-4 hover:underline",
};

const SIZES: Record<ButtonSize, string> = {
  default: "h-10 px-4 sm:h-9",
  sm: "h-8 px-3 text-xs",
  lg: "h-11 px-6",
  icon: "size-9",
};

/** Classes for a button look. Use them on a link that should look like a button. */
export function buttonClasses(variant: ButtonVariant = "default", size: ButtonSize = "default") {
  return cn(BASE, VARIANTS[variant], SIZES[size]);
}

export function Button(
  props: JSX.ButtonHTMLAttributes<HTMLButtonElement> & {
    variant?: ButtonVariant;
    size?: ButtonSize;
  },
) {
  const [own, rest] = splitProps(props, ["variant", "size", "class", "children"]);
  return (
    <button type="button" class={cn(buttonClasses(own.variant, own.size), own.class)} {...rest}>
      {own.children}
    </button>
  );
}
