import { For, Show } from "solid-js";
import { describeError } from "@/api/client";
import { useDomains } from "@/api/queries";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Link } from "@/lib/router";
import { buttonClasses } from "@/components/ui/button";
import { CardSkeleton } from "@/components/CardSkeleton";
import { MailPanel } from "@/components/MailPanel";
import { PageHeader } from "@/components/PageHeader";

/** Mailboxes for every connected domain. Turn mail on per domain, then add mailboxes. */
export function MailPage() {
  const domains = useDomains();
  return (
    <>
      <PageHeader
        title="Mail"
        description="Turn mail on for a domain, add mailboxes such as info@yourdomain.com, and copy the DNS records it needs."
      />
      <Show when={!domains.isPending} fallback={<CardSkeleton height="h-48" />}>
        <Show
          when={!domains.isError}
          fallback={
            <Alert variant="destructive">
              <AlertDescription>{describeError(domains.error)}</AlertDescription>
            </Alert>
          }
        >
          <Show
            when={(domains.data?.length ?? 0) > 0}
            fallback={
              <Card>
                <CardHeader>
                  <CardTitle>No domains yet</CardTitle>
                  <CardDescription>
                    Mail works on a domain you have connected and pointed at this server.
                  </CardDescription>
                </CardHeader>
                <CardContent>
                  <Link to="/domains" class={buttonClasses("default", "sm")}>
                    Connect a domain
                  </Link>
                </CardContent>
              </Card>
            }
          >
            <For each={domains.data}>
              {(domain) => (
                <Card>
                  <CardHeader>
                    <CardTitle>{domain.apex}</CardTitle>
                    <CardDescription>Mail addresses on {domain.apex}</CardDescription>
                  </CardHeader>
                  <CardContent>
                    <MailPanel domain={domain} />
                  </CardContent>
                </Card>
              )}
            </For>
          </Show>
        </Show>
      </Show>
    </>
  );
}
