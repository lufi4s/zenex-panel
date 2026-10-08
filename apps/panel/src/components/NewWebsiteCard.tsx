import { useState, type FormEvent } from "react";
import { useCreateSite, useDomains } from "@/api/queries";
import { ApiError } from "@/api/client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const LABEL_PATTERN = /^[a-z][a-z0-9-]{1,26}[a-z0-9]$/;

interface NewWebsiteCardProps {
  onCreated: (siteId: string, jobId: string) => void;
}

export function NewWebsiteCard({ onCreated }: NewWebsiteCardProps) {
  const domains = useDomains();
  const create = useCreateSite();
  const [apex, setApex] = useState<string>("");
  const [label, setLabel] = useState("");
  const [error, setError] = useState<string | null>(null);

  const selectedApex = apex || domains.data?.[0]?.apex || "";
  const cleanLabel = label.trim().toLowerCase();
  const labelValid = LABEL_PATTERN.test(cleanLabel);
  const canSubmit = labelValid && selectedApex !== "" && !create.isPending;

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!canSubmit) return;
    setError(null);
    create.mutate(
      { label: cleanLabel, apex: selectedApex },
      {
        onSuccess: (result) => {
          setLabel("");
          onCreated(result.site.id, result.job_id);
        },
        onError: (err) => {
          setError(err instanceof ApiError ? err.message : "Could not create the website.");
        },
      },
    );
  };

  const noDomain = domains.data !== undefined && domains.data.length === 0;

  return (
    <Card>
      <CardHeader>
        <CardTitle>New website</CardTitle>
        <CardDescription>
          Choose a name and the domain it lives on. WordPress is installed for you.
        </CardDescription>
      </CardHeader>
      <CardContent>
        {noDomain ? (
          <p className="text-sm text-muted-foreground">Add a domain above first.</p>
        ) : (
          <form
            onSubmit={submit}
            className="grid gap-4 sm:grid-cols-[1fr_1fr_auto] sm:items-end"
            noValidate
          >
            <div className="space-y-1.5">
              <Label htmlFor="site-label">Subdomain name</Label>
              <Input
                id="site-label"
                placeholder="shop"
                autoComplete="off"
                maxLength={28}
                value={label}
                onChange={(e) => setLabel(e.target.value)}
                aria-invalid={label !== "" && !labelValid ? true : undefined}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="site-apex">Domain</Label>
              <select
                id="site-apex"
                className="flex h-10 w-full rounded-md border border-input bg-background px-3 text-base sm:h-9 sm:text-sm focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring/30 disabled:opacity-50"
                value={selectedApex}
                onChange={(e) => setApex(e.target.value)}
                disabled={!domains.data || domains.data.length === 0}
              >
                {domains.data?.map((d) => (
                  <option key={d.id} value={d.apex}>
                    {d.verified ? d.apex : `${d.apex} (DNS not set)`}
                  </option>
                ))}
              </select>
            </div>
            <Button type="submit" disabled={!canSubmit}>
              {create.isPending ? "Creating…" : "Create website"}
            </Button>
            <p className="text-xs text-muted-foreground sm:col-span-3">
              {label !== "" && !labelValid
                ? "Use 3–28 lowercase letters, numbers or hyphens, starting with a letter."
                : labelValid && selectedApex
                  ? `Address: http://${cleanLabel}.${selectedApex}`
                  : " "}
            </p>
            {error && (
              <div className="sm:col-span-3">
                <Alert variant="destructive">
                  <AlertDescription>{error}</AlertDescription>
                </Alert>
              </div>
            )}
          </form>
        )}
      </CardContent>
    </Card>
  );
}
