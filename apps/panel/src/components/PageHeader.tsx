import { children as resolveChildren, Show, type JSX } from "solid-js";

/** Title row at the top of every page: a heading, a short description and actions. */
export function PageHeader(props: { title: string; description?: string; actions?: JSX.Element }) {
  // Resolve the actions once. Reading a JSX prop each time would build its content again.
  const actions = resolveChildren(() => props.actions);
  return (
    <div class="flex flex-col gap-4 sm:flex-row sm:items-end sm:justify-between">
      <div class="min-w-0 space-y-1">
        <h1 class="font-heading text-2xl font-semibold tracking-tight sm:text-3xl">
          {props.title}
        </h1>
        <Show when={props.description}>
          <p class="text-sm text-muted-foreground">{props.description}</p>
        </Show>
      </div>
      <Show when={actions()}>
        <div class="flex shrink-0 flex-wrap items-center gap-2">{actions()}</div>
      </Show>
    </div>
  );
}
