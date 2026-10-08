import { useState, type FormEvent } from "react";
import { AlertTriangle, CheckCircle2, RefreshCw, Trash2 } from "lucide-react";
import { useAddDomain, useCheckDomain, useDeleteDomain, useDomains } from "@/api/queries";
import { ApiError, describeError } from "@/api/client";
import type { Domain } from "@/api/types";
import { Alert, Badge } from "@/components/ui/badge-alert";
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
import { Input, Label } from "@/components/ui/input";

const APEX_PATTERN = /^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/;

function DeleteDomainDialog({ domain }: { domain: Domain }) {
  const remove = useDeleteDomain();
  const [open, setOpen] = useState(false);
  const [typed, setTyped] = useState("");
  const matches = typed.trim().toLowerCase() === domain.apex;

  const confirm = () => {
    if (!matches) return;
    remove.mutate(domain.id, {
      onSuccess: () => {
        setOpen(false);
        setTyped("");
      },
    });
  };

  return (
    <Dialog
      open={open}
      onOpenChange={(next) => {
        setOpen(next);
        if (!next) {
          setTyped("");
          remove.reset();
        }
      }}
    >
      <DialogTrigger asChild>
        <Button variant="ghost" size="sm" className="text-destructive hover:text-destructive">
          <Trash2 aria-hidden />
          Remove
        </Button>
      </DialogTrigger>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Remove {domain.apex}?</DialogTitle>
          <DialogDescription>
            The domain is removed from your account. Websites that still use it must be deleted
            first.
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-1.5">
          <Label htmlFor={`remove-${domain.id}`}>
            Type <span className="font-mono">{domain.apex}</span> to confirm
          </Label>
          <Input
            id={`remove-${domain.id}`}
            autoComplete="off"
            value={typed}
            onChange={(e) => setTyped(e.target.value)}
          />
        </div>
        {remove.isError && <Alert tone="danger">{describeError(remove.error)}</Alert>}
        <div className="flex justify-end gap-2">
          <Button variant="outline" onClick={() => setOpen(false)}>
            Cancel
          </Button>
          <Button variant="destructive" disabled={!matches || remove.isPending} onClick={confirm}>
            {remove.isPending ? "Removing…" : "Remove domain"}
          </Button>
        </div>
      </DialogContent>
    </Dialog>
  );
}

function DomainItem({ domain }: { domain: Domain }) {
  const check = useCheckDomain();
  return (
    <li className="flex flex-col gap-2 py-3 first:pt-0 last:pb-0">
      <div className="flex flex-wrap items-center gap-2">
        <span className="font-medium">{domain.apex}</span>
        {domain.verified ? (
          <Badge tone="success">
            <CheckCircle2 className="size-3" aria-hidden /> DNS ready
          </Badge>
        ) : (
          <Badge tone="warning">
            <AlertTriangle className="size-3" aria-hidden /> DNS not set
          </Badge>
        )}
        <div className="ml-auto flex items-center gap-1">
          <Button
            variant="ghost"
            size="sm"
            disabled={check.isPending}
            onClick={() => check.mutate(domain.id)}
          >
            <RefreshCw className={check.isPending ? "animate-spin" : undefined} aria-hidden />
            Check DNS
          </Button>
          <DeleteDomainDialog domain={domain} />
        </div>
      </div>
      {domain.message && (
        <p className={domain.verified ? "text-xs text-muted-foreground" : "text-xs text-warning"}>
          {domain.message}
        </p>
      )}
      {check.isError && <Alert tone="danger">{describeError(check.error)}</Alert>}
    </li>
  );
}

export function DomainsCard() {
  const domains = useDomains();
  const add = useAddDomain();
  const [apex, setApex] = useState("");
  const [error, setError] = useState<string | null>(null);

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    const value = apex.trim().toLowerCase();
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
      <CardContent className="space-y-4">
        {domains.isPending && <p className="text-sm text-muted-foreground">Loading…</p>}
        {domains.isError && <Alert tone="danger">{describeError(domains.error)}</Alert>}
        {domains.data && domains.data.length === 0 && (
          <p className="text-sm text-muted-foreground">No domain added yet.</p>
        )}
        {domains.data && domains.data.length > 0 && (
          <ul className="divide-y divide-border">
            {domains.data.map((d) => (
              <DomainItem key={d.id} domain={d} />
            ))}
          </ul>
        )}

        <form onSubmit={submit} className="flex flex-col gap-2 sm:flex-row sm:items-end" noValidate>
          <div className="flex-1 space-y-1.5">
            <Label htmlFor="apex">Domain</Label>
            <Input
              id="apex"
              placeholder="yourdomain.com"
              autoComplete="off"
              value={apex}
              onChange={(e) => setApex(e.target.value)}
              aria-invalid={error ? true : undefined}
              aria-describedby={error ? "apex-error" : undefined}
            />
          </div>
          <Button type="submit" disabled={add.isPending || apex.trim() === ""}>
            {add.isPending ? "Adding…" : "Add domain"}
          </Button>
        </form>
        {error && (
          <p id="apex-error" role="alert" className="text-sm text-destructive">
            {error}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
