import { useState } from "react";
import { ArrowUpCircle, CheckCircle2, RefreshCw, XCircle } from "lucide-react";
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
  const [confirming, setConfirming] = useState(false);

  if (status.isPending) return <CardSkeleton height="h-56" />;

  const data = status.data;
  const running = data?.state === "running" || start.isPending;
  const canUpdate = Boolean(data?.update_available) && !running;

  const begin = () => {
    start.mutate(undefined, {
      onSuccess: () => {
        setConfirming(false);
        toast.success("Update started. The panel will restart in a few minutes.");
      },
    });
  };

  const logLines = (data?.log ?? "")
    .split("\n")
    .filter((line) => line.length > 0)
    .map((text, i) => ({ id: i, text, level: levelFromText(text) }));

  return (
    <Card>
      <CardHeader>
        <CardTitle>Panel updates</CardTitle>
        <CardDescription>Install the newest version of the panel on this server.</CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <dl className="grid grid-cols-[auto_1fr] gap-x-6 gap-y-2 text-sm">
          <dt className="text-muted-foreground">Installed</dt>
          <dd className="font-mono">{data?.current || "unknown"}</dd>
          <dt className="text-muted-foreground">Latest</dt>
          <dd className="flex items-center gap-2 font-mono">
            {data?.latest || "unknown"}
            {data?.update_available && (
              <Badge variant="outline" className="font-sans">
                New version
              </Badge>
            )}
          </dd>
        </dl>

        {data?.error && (
          <Alert variant="destructive">
            <AlertDescription>{data.error}</AlertDescription>
          </Alert>
        )}
        {status.isError && (
          <Alert variant="destructive">
            <AlertDescription>{describeError(status.error)}</AlertDescription>
          </Alert>
        )}

        {running && (
          <p className="flex items-center gap-2 text-sm text-muted-foreground">
            <RefreshCw className="size-4 animate-spin" aria-hidden />
            Updating. This takes a few minutes; the panel may disconnect briefly.
          </p>
        )}
        {!running && data?.state === "succeeded" && (
          <p className="flex items-center gap-2 text-sm text-success">
            <CheckCircle2 className="size-4" aria-hidden /> The last update finished successfully.
          </p>
        )}
        {!running && data?.state === "failed" && (
          <p className="flex items-center gap-2 text-sm text-destructive">
            <XCircle className="size-4" aria-hidden /> The last update failed. See the log below.
          </p>
        )}
        {start.isError && (
          <Alert variant="destructive">
            <AlertDescription>{describeError(start.error)}</AlertDescription>
          </Alert>
        )}

        <Dialog open={confirming} onOpenChange={setConfirming}>
          <DialogTrigger asChild>
            <Button disabled={!canUpdate}>
              <ArrowUpCircle aria-hidden /> Update now
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

        {(running || data?.state === "failed" || data?.state === "succeeded") &&
          logLines.length > 0 && (
            <Terminal
              title="update log"
              emptyText="No output yet."
              maxHeight="18rem"
              lines={logLines}
            />
          )}
      </CardContent>
    </Card>
  );
}
