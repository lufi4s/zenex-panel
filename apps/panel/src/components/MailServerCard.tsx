import { Show } from "solid-js";
import { describeError } from "@/api/client";
import { useApplyMailServer } from "@/api/queries";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";

/** Writes the mail server configuration to the server and reloads it. Administrators only. */
export function MailServerCard() {
  const apply = useApplyMailServer();
  return (
    <Card>
      <CardHeader>
        <CardTitle>Mail server</CardTitle>
        <CardDescription>
          Applies the mail settings of every hosted domain to the server. The server checks the new
          configuration first and keeps the old one if the check fails.
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <div class="flex flex-wrap items-center gap-3">
          <Button disabled={apply.isPending} onClick={() => apply.mutate(undefined)}>
            {apply.isPending ? "Applying…" : "Apply mail configuration"}
          </Button>
          <Show when={apply.isSuccess}>
            <span class="text-sm text-success">Mail configuration applied.</span>
          </Show>
        </div>
        <Show when={apply.isError}>
          <Alert variant="destructive">
            <AlertDescription>{describeError(apply.error)}</AlertDescription>
          </Alert>
        </Show>
      </CardContent>
    </Card>
  );
}
