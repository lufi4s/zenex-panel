import { createEffect, createMemo, createSignal, For, onCleanup, Show } from "solid-js";
import { useActivity, useSites, type ActivityFilter } from "@/api/queries";
import type { ActivityResult, ActivityRow } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { badgeTone } from "@/lib/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { cn } from "@/lib/utils";

const RESULTS: { value: ActivityResult | ""; label: string }[] = [
  { value: "", label: "All" },
  { value: "success", label: "Success" },
  { value: "failure", label: "Failed" },
  { value: "denied", label: "Denied" },
];

const RESULT_TONE: Record<ActivityResult, "success" | "danger" | "warning"> = {
  success: "success",
  failure: "danger",
  denied: "warning",
};

/** Readable names for the actions the panel records. Unknown actions show as-is. */
const ACTION_LABEL: Record<string, string> = {
  "login.success": "Signed in",
  "login.failure": "Sign-in failed",
  "login.locked": "Sign-in blocked (locked)",
  logout: "Signed out",
  "domain.connect": "Added domain",
  "domain.verify": "Checked DNS",
  "site.create": "Created website",
  "site.delete": "Deleted website",
  "site.suspend": "Took website offline",
  "site.resume": "Brought website online",
  "site.php_restart": "Restarted PHP",
  "site.php_switch": "Changed PHP version",
  "site.credentials.view": "Viewed WordPress login",
  "job.retry": "Retried build",
};

function when(iso: string): string {
  return new Date(iso).toLocaleString(undefined, {
    month: "short",
    day: "numeric",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function Row(props: { row: ActivityRow; siteNames: Map<string, string> }) {
  const target = () =>
    props.row.target_type === "site" && props.row.target_id
      ? (props.siteNames.get(props.row.target_id) ?? "Website")
      : props.row.target_type === "domain" && props.row.target_id
        ? props.row.target_id
        : "";
  return (
    <tr class="border-b border-border last:border-0">
      <td class="whitespace-nowrap py-2 pr-3 text-xs text-muted-foreground tabular-nums">
        {when(props.row.time)}
      </td>
      <td class="py-2 pr-3">
        <div class="font-medium">{ACTION_LABEL[props.row.action] ?? props.row.action}</div>
        <Show when={target()}>
          <div class="truncate text-xs text-muted-foreground">{target()}</div>
        </Show>
      </td>
      <td class="py-2 pr-3">
        <Badge variant="outline" class={badgeTone(RESULT_TONE[props.row.result])}>
          {props.row.result}
        </Badge>
      </td>
      <td class="hidden whitespace-nowrap py-2 pr-3 text-xs text-muted-foreground md:table-cell">
        {props.row.error_code ? props.row.error_code : ""}
      </td>
      <td class="hidden whitespace-nowrap py-2 text-xs text-muted-foreground sm:table-cell">
        {props.row.ip ?? ""}
      </td>
    </tr>
  );
}

/**
 * The rows for one filter. A new filter (or Live toggle) mounts a new feed,
 * because the activity query reads its filter once when it starts.
 */
function ActivityFeed(props: {
  filter: ActivityFilter;
  live: boolean;
  siteNames: Map<string, string>;
}) {
  const feed = useActivity(props.filter, props.live);
  const rows = () => feed.data?.pages.flatMap((p) => p.items) ?? [];

  return (
    <>
      <Show when={feed.isError}>
        <Alert variant="destructive">
          <AlertDescription>Could not load activity.</AlertDescription>
        </Alert>
      </Show>
      <Show when={feed.isPending}>
        <p class="text-sm text-muted-foreground">Loading…</p>
      </Show>
      <Show when={feed.data && rows().length === 0}>
        <p class="text-sm text-muted-foreground">Nothing matches these filters.</p>
      </Show>

      <Show when={rows().length > 0}>
        <div class="overflow-x-auto">
          <table class="w-full text-left text-sm">
            <thead>
              <tr class="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                <th class="py-2 pr-3 font-medium">Time</th>
                <th class="py-2 pr-3 font-medium">Action</th>
                <th class="py-2 pr-3 font-medium">Result</th>
                <th class="hidden py-2 pr-3 font-medium md:table-cell">Reason</th>
                <th class="hidden py-2 font-medium sm:table-cell">IP</th>
              </tr>
            </thead>
            <tbody>
              <For each={rows()}>{(row) => <Row row={row} siteNames={props.siteNames} />}</For>
            </tbody>
          </table>
        </div>
      </Show>

      <Show when={feed.hasNextPage}>
        <Button
          variant="outline"
          size="sm"
          disabled={feed.isFetchingNextPage}
          onClick={() => feed.fetchNextPage()}
        >
          {feed.isFetchingNextPage ? "Loading…" : "Load older"}
        </Button>
      </Show>
    </>
  );
}

/**
 * Every recorded action, newest first. Filter by text or result, and load older
 * entries on demand. Refreshes while the page is open.
 */
export function ActivityCard() {
  const [search, setSearch] = createSignal("");
  const [debounced, setDebounced] = createSignal("");
  const [result, setResult] = createSignal<ActivityResult | "">("");
  const [live, setLive] = createSignal(true);
  const sites = useSites();

  createEffect(() => {
    const value = search().trim();
    const id = setTimeout(() => setDebounced(value), 300);
    onCleanup(() => clearTimeout(id));
  });

  const query = createMemo(() => ({ action: debounced(), result: result(), live: live() }));
  const siteNames = createMemo(
    () => new Map((sites.data ?? []).map((s) => [s.id, s.domain] as const)),
  );

  return (
    <Card>
      <CardHeader class="flex-row flex-wrap items-start justify-between gap-3">
        <div>
          <CardTitle>Activity</CardTitle>
          <CardDescription>Everything that happened on your account, newest first.</CardDescription>
        </div>
        <Button
          variant="outline"
          size="sm"
          aria-pressed={live()}
          onClick={() => setLive((v) => !v)}
        >
          <span
            class={cn(
              "size-2 rounded-full",
              live() ? "animate-pulse bg-success" : "bg-muted-foreground",
            )}
            aria-hidden="true"
          />
          {live() ? "Live" : "Paused"}
        </Button>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="flex flex-wrap items-center gap-2">
          <Input
            class="w-full sm:max-w-xs"
            placeholder="Search actions, e.g. site or login"
            value={search()}
            onInput={(e) => setSearch(e.currentTarget.value)}
            aria-label="Search activity"
          />
          <div
            role="group"
            aria-label="Filter by result"
            class="inline-flex rounded-md border border-border p-0.5"
          >
            <For each={RESULTS}>
              {(r) => (
                <button
                  type="button"
                  aria-pressed={result() === r.value}
                  onClick={() => setResult(r.value)}
                  class={cn(
                    "rounded-sm px-2.5 py-1 text-xs font-medium",
                    result() === r.value
                      ? "bg-primary text-primary-foreground"
                      : "text-muted-foreground hover:text-foreground",
                  )}
                >
                  {r.label}
                </button>
              )}
            </For>
          </div>
        </div>

        <Show when={query()} keyed>
          {(q) => (
            <ActivityFeed
              filter={{ action: q.action, result: q.result }}
              live={q.live}
              siteNames={siteNames()}
            />
          )}
        </Show>
      </CardContent>
    </Card>
  );
}
