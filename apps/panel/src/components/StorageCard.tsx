import { For, Show } from "solid-js";
import { describeError } from "@/api/client";
import { useStorage } from "@/api/queries";
import type { SiteStorage } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { formatBytes } from "@/lib/format";

/** A thin bar showing value as a share of max. */
function Bar(props: { value: number; max: number }) {
  const pct = () => (props.max > 0 ? Math.min(100, (props.value / props.max) * 100) : 0);
  return (
    <div class="h-1.5 w-full rounded-full bg-muted">
      <div class="h-full rounded-full bg-primary" style={{ width: `${pct()}%` }} />
    </div>
  );
}

/** Disk use per website: files, database and backups. Administrators also see the server disk. */
export function StorageCard() {
  const storage = useStorage();
  const largest = () => Math.max(0, ...(storage.data?.sites ?? []).map((s) => s.total_bytes));
  const totals = () => {
    const sites = storage.data?.sites ?? [];
    const sum = (pick: (s: SiteStorage) => number) => sites.reduce((acc, s) => acc + pick(s), 0);
    return {
      files: sum((s) => s.files_bytes),
      database: sum((s) => s.database_bytes),
      backups: sum((s) => s.backup_bytes),
      total: sum((s) => s.total_bytes),
    };
  };

  return (
    <Show
      when={storage.data}
      fallback={
        // Nothing is drawn while the measurement loads, so the page does not jump.
        <Show when={storage.isError}>
          <Alert variant="destructive">
            <AlertDescription>{describeError(storage.error)}</AlertDescription>
          </Alert>
        </Show>
      }
    >
      {(report) => (
        <Card>
          <CardHeader>
            <CardTitle>Storage</CardTitle>
            <CardDescription>
              How much disk each website uses: files, database and backups.
            </CardDescription>
          </CardHeader>
          <CardContent class="space-y-5">
            <Show when={report().disk}>
              {(disk) => (
                <div class="space-y-2 text-sm">
                  <div class="flex flex-wrap justify-between gap-2">
                    <span class="font-medium">Server disk</span>
                    <span class="tabular-nums text-muted-foreground">
                      {formatBytes(disk().used_bytes)} of {formatBytes(disk().total_bytes)} used
                    </span>
                  </div>
                  <Bar value={disk().used_bytes} max={disk().total_bytes} />
                </div>
              )}
            </Show>

            <div class="overflow-x-auto rounded-lg border border-border">
              <table class="w-full text-sm">
                <thead class="bg-muted/50 text-left text-xs uppercase tracking-wide text-muted-foreground">
                  <tr>
                    <th class="px-4 py-2 font-medium">Website</th>
                    <th class="px-4 py-2 font-medium">Files</th>
                    <th class="px-4 py-2 font-medium">Database</th>
                    <th class="px-4 py-2 font-medium">Backups</th>
                    <th class="px-4 py-2 font-medium">Total</th>
                  </tr>
                </thead>
                <tbody>
                  <For each={report().sites}>
                    {(site) => (
                      <tr class="border-t border-border">
                        <td class="px-4 py-2 font-medium">{site.domain}</td>
                        <td class="px-4 py-2 tabular-nums">{formatBytes(site.files_bytes)}</td>
                        <td class="px-4 py-2 tabular-nums">{formatBytes(site.database_bytes)}</td>
                        <td class="px-4 py-2 tabular-nums">{formatBytes(site.backup_bytes)}</td>
                        <td class="w-56 px-4 py-2">
                          <div class="space-y-1">
                            <span class="tabular-nums">{formatBytes(site.total_bytes)}</span>
                            <Bar value={site.total_bytes} max={largest()} />
                          </div>
                        </td>
                      </tr>
                    )}
                  </For>
                  <tr class="border-t border-border bg-muted/30 font-semibold">
                    <td class="px-4 py-2">All websites</td>
                    <td class="px-4 py-2 tabular-nums">{formatBytes(totals().files)}</td>
                    <td class="px-4 py-2 tabular-nums">{formatBytes(totals().database)}</td>
                    <td class="px-4 py-2 tabular-nums">{formatBytes(totals().backups)}</td>
                    <td class="px-4 py-2 tabular-nums">{formatBytes(totals().total)}</td>
                  </tr>
                </tbody>
              </table>
            </div>
            <p class="text-xs text-muted-foreground">
              Figures are refreshed about every five minutes. Websites that have never been backed
              up show 0 B.
            </p>
          </CardContent>
        </Card>
      )}
    </Show>
  );
}
