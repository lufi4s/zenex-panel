import { useState, type FormEvent } from "react";
import { CheckCircle2, AlertTriangle, RefreshCw } from "lucide-react";
import { useAddDomain, useCheckDomain, useDomains } from "@/api/queries";
import { errorMessageFrom, ApiError } from "@/api/client";
import type { Domain } from "@/api/types";
import { Alert, Badge } from "@/components/ui/badge-alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input, Label } from "@/components/ui/input";

const APEX_PATTERN = /^(?=.{1,253}$)([a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?\.)+[a-z]{2,63}$/;

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
        <Button
          variant="ghost"
          size="sm"
          className="ml-auto"
          disabled={check.isPending}
          onClick={() => check.mutate(domain.id)}
        >
          <RefreshCw className={check.isPending ? "animate-spin" : undefined} aria-hidden />
          Check DNS
        </Button>
      </div>
      {domain.message && (
        <p className={domain.verified ? "text-xs text-muted-foreground" : "text-xs text-warning"}>
          {domain.message}
        </p>
      )}
      {check.isError && (
        <Alert tone="danger">{errorMessageFrom(null, "Could not check DNS. Try again.")}</Alert>
      )}
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
      setError("Enter a domain such as yourdomain.com");
      return;
    }
    setError(null);
    add.mutate(value, {
      onSuccess: () => setApex(""),
      onError: (err) => {
        setError(err instanceof ApiError ? err.message : "Could not add the domain.");
      },
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
        {domains.isError && <Alert tone="danger">Could not load your domains.</Alert>}
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
          <p id="apex-error" className="text-sm text-destructive">
            {error}
          </p>
        )}
      </CardContent>
    </Card>
  );
}
