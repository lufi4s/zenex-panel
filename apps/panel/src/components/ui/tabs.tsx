import { Tabs as KobalteTabs } from "@kobalte/core/tabs";
import type { JSX } from "solid-js";
import { cn } from "@/lib/utils";

/** Tabs (Kobalte: keyboard arrows, roles and focus handled for us). */
export function Tabs(props: {
  value?: string;
  defaultValue?: string;
  onValueChange?: (value: string) => void;
  class?: string;
  children: JSX.Element;
}) {
  return (
    <KobalteTabs
      value={props.value}
      defaultValue={props.defaultValue}
      onChange={(value) => props.onValueChange?.(value)}
      class={cn("flex flex-col gap-4", props.class)}
    >
      {props.children}
    </KobalteTabs>
  );
}

export function TabsList(props: { class?: string; children: JSX.Element }) {
  return (
    <KobalteTabs.List
      class={cn(
        "inline-flex w-full items-center gap-1 overflow-x-auto rounded-lg border border-border bg-muted/50 p-1",
        props.class,
      )}
    >
      {props.children}
    </KobalteTabs.List>
  );
}

export function TabsTrigger(props: { value: string; class?: string; children: JSX.Element }) {
  return (
    <KobalteTabs.Trigger
      value={props.value}
      class={cn(
        "inline-flex items-center justify-center whitespace-nowrap rounded-md px-3 py-1.5 text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/40",
        "text-muted-foreground hover:text-foreground data-[selected]:bg-card data-[selected]:text-foreground data-[selected]:shadow-sm",
        props.class,
      )}
    >
      {props.children}
    </KobalteTabs.Trigger>
  );
}

/** Renders its content only while its tab is selected. */
export function TabsContent(props: { value: string; class?: string; children: JSX.Element }) {
  return (
    <KobalteTabs.Content value={props.value} class={props.class}>
      {props.children}
    </KobalteTabs.Content>
  );
}
