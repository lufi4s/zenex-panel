import { createMemo, createSignal, Show } from "solid-js";
import { ArrowUpCircle, CheckCircle2, RefreshCw, XCircle } from "@/components/icons";
import { useStartUpdate, useSystemUpdate } from "@/api/queries";
import { describeError } from "@/api/client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { CardSkeleton } from "@/components/CardSkeleton";
import { Terminal, levelFromText } from "@/components/Terminal";
import { toast } from "@/lib/toast";

/** Panel version, the latest release, and a one-click update for administrators. */
export function UpdateCard() {
  const status = useSystemUpdate(true);
  const start = useStartUpdate();
  const [confirming, setConfirming] = createSignal(false);

  const running = () => status.data?.state === "running" || start.isPending;
  const canUpdate = () => Boolean(status.data?.update_available) && !running();

  const begin = () => {
    start.mutate(undefined, {
      onSuccess: () => {
        setConfirming(false);
        toast.success("Update started. The panel will restart in a few minutes.");
      },
    });
  };

  const logLines = createMemo(() =>
    (status.data?.log ?? "")
      .split("\n")
      .filter((line) => line.length > 0)
      .map((text, i) => ({ id: i, text, level: levelFromText(text) })),
  );

  return (
    <Show when={!status.isPending} fallback={<CardSkeleton height="h-56" />}>
      <Card>
        <CardHeader>
          <CardTitle>Panel updates</CardTitle>
          <CardDescription>Install the newest version of the panel on this server.</CardDescription>
        </CardHeader>
        <CardContent class="space-y-4">
          <dl class="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
            <dt class="text-muted-foreground">Installed</dt>
            <dd class="font-mono">{status.data?.current || "unknown"}</dd>
            <dt class="text-muted-foreground">Latest</dt>
            <dd class="flex items-center gap-2 font-mono">
              {status.data?.latest || "unknown"}
              <Show when={status.data?.update_available}>
                <Badge variant="outline" class="font-sans">
                  New version
                </Badge>
              </Show>
            </dd>
          </dl>

          <Show when={status.data?.error}>
            {(message) => (
              <Alert variant="destructive">
                <AlertDescription>{message()}</AlertDescription>
              </Alert>
            )}
          </Show>
          <Show when={status.isError}>
            <Alert variant="destructive">
              <AlertDescription>{describeError(status.error)}</AlertDescription>
            </Alert>
          </Show>

          <Show when={running()}>
            <p class="flex items-center gap-2 text-sm text-muted-foreground">
              <RefreshCw class="size-4 animate-spin" aria-hidden="true" />
              Updating. This takes a few minutes; the panel may disconnect briefly.
            </p>
          </Show>
          <Show when={!running() && status.data?.state === "succeeded"}>
            <p class="flex items-center gap-2 text-sm text-success">
              <CheckCircle2 class="size-4" aria-hidden="true" /> The last update finished
              successfully.
            </p>
          </Show>
          <Show when={!running() && status.data?.state === "failed"}>
            <p class="flex items-center gap-2 text-sm text-destructive">
              <XCircle class="size-4" aria-hidden="true" /> The last update failed. See the log
              below.
            </p>
          </Show>
          <Show when={start.isError}>
            <Alert variant="destructive">
              <AlertDescription>{describeError(start.error)}</AlertDescription>
            </Alert>
          </Show>

          <Dialog open={confirming()} onOpenChange={setConfirming}>
            <DialogTrigger>
              <Button disabled={!canUpdate()}>
                <ArrowUpCircle aria-hidden="true" /> Update now
              </Button>
            </DialogTrigger>
            <DialogContent>
              <DialogHeader>
                <DialogTitle>Update the panel?</DialogTitle>
                <DialogDescription>
                  The newest version is installed and the panel restarts. Websites keep running. You
                  may be signed out for a minute.
                </DialogDescription>
              </DialogHeader>
              <DialogFooter>
                <Button variant="outline" onClick={() => setConfirming(false)}>
                  Cancel
                </Button>
                <Button disabled={start.isPending} onClick={begin}>
                  {start.isPending ? "Starting…" : "Update panel"}
                </Button>
              </DialogFooter>
            </DialogContent>
          </Dialog>

          <Show
            when={
              (running() ||
                status.data?.state === "failed" ||
                status.data?.state === "succeeded") &&
              logLines().length > 0
            }
          >
            <Terminal
              title="update log"
              emptyText="No output yet."
              maxHeight="18rem"
              lines={logLines()}
            />
          </Show>
        </CardContent>
      </Card>
    </Show>
  );
}
