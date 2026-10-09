import { Select } from "@/components/ui/select";
import { createSignal, For, Show } from "solid-js";
import { useCreateSite, useDomains } from "@/api/queries";
import { ApiError } from "@/api/client";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

const LABEL_PATTERN = /^[a-z][a-z0-9-]{1,26}[a-z0-9]$/;

export function NewWebsiteCard(props: { onCreated: (siteId: string, jobId: string) => void }) {
  const domains = useDomains();
  const create = useCreateSite();
  const [apex, setApex] = createSignal<string>("");
  const [label, setLabel] = createSignal("");
  const [error, setError] = createSignal<string | null>(null);

  const selectedApex = () => apex() || domains.data?.[0]?.apex || "";
  const cleanLabel = () => label().trim().toLowerCase();
  const labelValid = () => LABEL_PATTERN.test(cleanLabel());
  const canSubmit = () => labelValid() && selectedApex() !== "" && !create.isPending;
  const noDomain = () => domains.data !== undefined && domains.data.length === 0;

  const submit = (event: SubmitEvent) => {
    event.preventDefault();
    if (!canSubmit()) return;
    setError(null);
    create.mutate(
      { label: cleanLabel(), apex: selectedApex() },
      {
        onSuccess: (result) => {
          setLabel("");
          props.onCreated(result.site.id, result.job_id);
        },
        onError: (err) => {
          setError(err instanceof ApiError ? err.message : "Could not create the website.");
        },
      },
    );
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>New website</CardTitle>
        <CardDescription>
          Choose a name and the domain it lives on. WordPress is installed for you.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <Show
          when={!noDomain()}
          fallback={<p class="text-sm text-muted-foreground">Add a domain above first.</p>}
        >
          <form onSubmit={submit} class="grid gap-5" noValidate>
            <div class="space-y-1.5">
              <Label for="site-label">Subdomain name</Label>
              <Input
                id="site-label"
                placeholder="shop"
                autocomplete="off"
                maxLength={28}
                value={label()}
                onInput={(e) => setLabel(e.currentTarget.value)}
                aria-invalid={label() !== "" && !labelValid() ? true : undefined}
              />
            </div>
            <div class="space-y-1.5">
              <Label for="site-apex">Domain</Label>
              <Select
                id="site-apex"
                value={selectedApex()}
                onChange={(e) => setApex(e.currentTarget.value)}
                disabled={!domains.data || domains.data.length === 0}
              >
                <For each={domains.data ?? []}>
                  {(d) => (
                    <option value={d.apex}>
                      {d.verified ? d.apex : `${d.apex} (DNS not set)`}
                    </option>
                  )}
                </For>
              </Select>
            </div>
            <Button
              type="submit"
              class="w-full sm:w-auto sm:justify-self-end"
              disabled={!canSubmit()}
            >
              {create.isPending ? "Creating…" : "Create website"}
            </Button>
            <p class="text-xs text-muted-foreground">
              {label() !== "" && !labelValid()
                ? "Use 3–28 lowercase letters, numbers or hyphens, starting with a letter."
                : labelValid() && selectedApex()
                  ? `Address: https://${cleanLabel()}.${selectedApex()}`
                  : " "}
            </p>
            <Show when={error()}>
              {(message) => (
                <div>
                  <Alert variant="destructive">
                    <AlertDescription>{message()}</AlertDescription>
                  </Alert>
                </div>
              )}
            </Show>
          </form>
        </Show>
      </CardContent>
    </Card>
  );
}
