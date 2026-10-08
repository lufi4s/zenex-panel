import { useEffect, useMemo, useState } from "react";
import { useActivity, useSites } from "@/api/queries";
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

function Row({ row, siteNames }: { row: ActivityRow; siteNames: Map<string, string> }) {
  const target =
    row.target_type === "site" && row.target_id
      ? (siteNames.get(row.target_id) ?? "Website")
      : row.target_type === "domain" && row.target_id
        ? row.target_id
        : "";
  return (
    <tr className="border-b border-border last:border-0">
      <td className="whitespace-nowrap py-2 pr-3 text-xs text-muted-foreground tabular-nums">
        {when(row.time)}
      </td>
      <td className="py-2 pr-3">
        <div className="font-medium">{ACTION_LABEL[row.action] ?? row.action}</div>
        {target && <div className="truncate text-xs text-muted-foreground">{target}</div>}
      </td>
      <td className="py-2 pr-3">
        <Badge variant="outline" className={badgeTone(RESULT_TONE[row.result])}>
          {row.result}
        </Badge>
      </td>
      <td className="hidden whitespace-nowrap py-2 pr-3 text-xs text-muted-foreground md:table-cell">
        {row.error_code ? row.error_code : ""}
      </td>
      <td className="hidden whitespace-nowrap py-2 text-xs text-muted-foreground sm:table-cell">
        {row.ip ?? ""}
      </td>
    </tr>
  );
}

/**
 * Every recorded action, newest first. Filter by text or result, and load older
 * entries on demand. Refreshes while the page is open.
 */
export function ActivityCard() {
  const [search, setSearch] = useState("");
  const [debounced, setDebounced] = useState("");
  const [result, setResult] = useState<ActivityResult | "">("");
  const [live, setLive] = useState(true);
  const sites = useSites();

  useEffect(() => {
    const id = setTimeout(() => setDebounced(search.trim()), 300);
    return () => clearTimeout(id);
  }, [search]);

  const feed = useActivity({ action: debounced, result }, live);
  const rows = useMemo(() => feed.data?.pages.flatMap((p) => p.items) ?? [], [feed.data]);
  const siteNames = useMemo(
    () => new Map((sites.data ?? []).map((s) => [s.id, s.domain])),
    [sites.data],
  );

  return (
    <Card>
      <CardHeader className="flex-row flex-wrap items-start justify-between gap-3">
        <div>
          <CardTitle>Activity</CardTitle>
          <CardDescription>Everything that happened on your account, newest first.</CardDescription>
        </div>
        <Button variant="outline" size="sm" aria-pressed={live} onClick={() => setLive((v) => !v)}>
          <span
            className={cn(
              "size-2 rounded-full",
              live ? "animate-pulse bg-success" : "bg-muted-foreground",
            )}
            aria-hidden
          />
          {live ? "Live" : "Paused"}
        </Button>
      </CardHeader>
      <CardContent className="space-y-4">
        <div className="flex flex-wrap items-center gap-2">
          <Input
            className="w-full sm:max-w-xs"
            placeholder="Search actions, e.g. site or login"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            aria-label="Search activity"
          />
          <div
            role="group"
            aria-label="Filter by result"
            className="inline-flex rounded-md border border-border p-0.5"
          >
            {RESULTS.map((r) => (
              <button
                key={r.label}
                type="button"
                aria-pressed={result === r.value}
                onClick={() => setResult(r.value)}
                className={cn(
                  "rounded-sm px-2.5 py-1 text-xs font-medium",
                  result === r.value
                    ? "bg-primary text-primary-foreground"
                    : "text-muted-foreground hover:text-foreground",
                )}
              >
                {r.label}
              </button>
            ))}
          </div>
        </div>

        {feed.isError && (
          <Alert variant="destructive">
            <AlertDescription>Could not load activity.</AlertDescription>
          </Alert>
        )}
        {feed.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}
        {feed.data && rows.length === 0 && (
          <p className="text-sm text-muted-foreground">Nothing matches these filters.</p>
        )}

        {rows.length > 0 && (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-sm">
              <thead>
                <tr className="border-b border-border text-xs uppercase tracking-wide text-muted-foreground">
                  <th className="py-2 pr-3 font-medium">Time</th>
                  <th className="py-2 pr-3 font-medium">Action</th>
                  <th className="py-2 pr-3 font-medium">Result</th>
                  <th className="hidden py-2 pr-3 font-medium md:table-cell">Reason</th>
                  <th className="hidden py-2 font-medium sm:table-cell">IP</th>
                </tr>
              </thead>
              <tbody>
                {rows.map((row) => (
                  <Row key={row.id} row={row} siteNames={siteNames} />
                ))}
              </tbody>
            </table>
          </div>
        )}

        {feed.hasNextPage && (
          <Button
            variant="outline"
            size="sm"
            disabled={feed.isFetchingNextPage}
            onClick={() => feed.fetchNextPage()}
          >
            {feed.isFetchingNextPage ? "Loading…" : "Load older"}
          </Button>
        )}
      </CardContent>
    </Card>
  );
}
