import { createSignal, For, Show } from "solid-js";
import { AlertTriangle, CheckCircle2, RefreshCw, Trash2 } from "@/components/icons";
import { useAddDomain, useCheckDomain, useDeleteDomain, useDomains } from "@/api/queries";
import { ApiError, describeError } from "@/api/client";
import type { Domain } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
  DialogTrigger,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const APEX_PATTERN = /^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/;

function DeleteDomainDialog(props: { domain: Domain }) {
  const remove = useDeleteDomain();
  const [open, setOpen] = createSignal(false);
  const [typed, setTyped] = createSignal("");
  const matches = () => typed().trim().toLowerCase() === props.domain.apex;

  const confirm = () => {
    if (!matches()) return;
    remove.mutate(props.domain.id, {
      onSuccess: () => {
        setOpen(false);
        setTyped("");
      },
    });
  };

  return (
    <Dialog
      open={open()}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setTyped("");
          remove.reset();
        }
      }}
    >
      <DialogTrigger>
        <Button variant="ghost" size="sm" class="text-destructive hover:text-destructive">
          <Trash2 aria-hidden="true" />
          Remove
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Remove {props.domain.apex}?</DialogTitle>
          <DialogDescription>
            The domain is removed from your account. Websites that still use it must be deleted
            first.
          </DialogDescription>
        </DialogHeader>
        <div class="space-y-1.5">
          <Label for={`remove-${props.domain.id}`}>
            Type <span class="font-mono">{props.domain.apex}</span> to confirm
          </Label>
          <Input
            id={`remove-${props.domain.id}`}
            autocomplete="off"
            value={typed()}
            onInput={(e) => setTyped(e.currentTarget.value)}
          />
        </div>
        <Show when={remove.isError}>
          <Alert variant="destructive">
            <AlertDescription>{describeError(remove.error)}</AlertDescription>
          </Alert>
        </Show>
        <div class="flex justify-end gap-2">
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button variant="destructive" disabled={!matches() || remove.isPending} onClick={confirm}>
            {remove.isPending ? "Removing…" : "Remove domain"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function DomainItem(props: { domain: Domain }) {
  const check = useCheckDomain();
  return (
    <li class="flex flex-col gap-2 py-3 first:pt-0 last:pb-0">
      <div class="flex flex-wrap items-center gap-2">
        <span class="font-medium">{props.domain.apex}</span>
        <Show
          when={props.domain.verified}
          fallback={
            <Badge variant="outline" class="border-warning/40 bg-warning/10 text-warning">
              <AlertTriangle class="size-3" aria-hidden="true" /> DNS not set
            </Badge>
          }
        >
          <Badge variant="outline" class="border-success/40 bg-success/10 text-success">
            <CheckCircle2 class="size-3" aria-hidden="true" /> DNS ready
          </Badge>
        </Show>
        <div class="ml-auto flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            disabled={check.isPending}
            onClick={() => check.mutate(props.domain.id)}
          >
            <RefreshCw class={check.isPending ? "animate-spin" : undefined} aria-hidden="true" />
            Check DNS
          </Button>
          <DeleteDomainDialog domain={props.domain} />
        </div>
      </div>
      <Show when={props.domain.message}>
        <p class={props.domain.verified ? "text-xs text-muted-foreground" : "text-xs text-warning"}>
          {props.domain.message}
        </p>
      </Show>
      <Show when={check.isError}>
        <Alert variant="destructive">
          <AlertDescription>{describeError(check.error)}</AlertDescription>
        </Alert>
      </Show>
    </li>
  );
}

export function DomainsCard() {
  const domains = useDomains();
  const add = useAddDomain();
  const [apex, setApex] = createSignal("");
  const [error, setError] = createSignal<string | null>(null);

  const submit = (event: SubmitEvent) => {
    event.preventDefault();
    const value = apex().trim().toLowerCase();
    if (!APEX_PATTERN.test(value)) {
      setError("Enter a domain such as yourdomain.com. Do not include http:// or a slash.");
      return;
    }
    setError(null);
    add.mutate(value, {
      onSuccess: () => setApex(""),
      onError: (err) => setError(err instanceof ApiError ? err.message : describeError(err)),
    });
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>Your domain</CardTitle>
        <CardDescription>
          Add the domain your websites will use. Then point a wildcard DNS record (<code>*</code>)
          to this server. The panel checks it for you.
        </CardDescription>
      </CardHeader>
      <CardContent class="space-y-4">
        <Show when={domains.isPending}>
          <p class="text-sm text-muted-foreground">Loading…</p>
        </Show>
        <Show when={domains.isError}>
          <Alert variant="destructive">
            <AlertDescription>{describeError(domains.error)}</AlertDescription>
          </Alert>
        </Show>
        <Show when={domains.data && domains.data.length === 0}>
          <p class="text-sm text-muted-foreground">No domain added yet.</p>
        </Show>
        <Show when={domains.data && domains.data.length > 0 ? domains.data : undefined}>
          {(list) => (
            <ul class="divide-y divide-border">
              <For each={list()}>{(d) => <DomainItem domain={d} />}</For>
            </ul>
          )}
        </Show>

        <form onSubmit={submit} class="flex flex-col gap-2 sm:flex-row sm:items-end" noValidate>
          <div class="flex-1 space-y-1.5">
            <Label for="apex">Domain</Label>
            <Input
              id="apex"
              placeholder="yourdomain.com"
              autocomplete="off"
              value={apex()}
              onInput={(e) => setApex(e.currentTarget.value)}
              aria-invalid={error() ? true : undefined}
              aria-describedby={error() ? "apex-error" : undefined}
            />
          </div>
          <Button type="submit" disabled={add.isPending || apex().trim() === ""}>
            {add.isPending ? "Adding…" : "Add domain"}
          </Button>
        </form>
        <Show when={error()}>
          {(message) => (
            <p id="apex-error" role="alert" class="text-sm text-destructive">
              {message()}
            </p>
          )}
        </Show>
      </CardContent>
    </Card>
  );
}
