import { createSignal, For, Show } from "solid-js";
import { describeError } from "@/api/client";
import {
  useAddMailbox,
  useChangeMailboxPassword,
  useCheckMailDNS,
  useMail,
  useRemoveMailbox,
  useSetMail,
} from "@/api/queries";
import type { Domain, MailCheckResult } from "@/api/types";
import { Check, Copy, Trash2 } from "@/components/icons";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button, buttonClasses } from "@/components/ui/button";
import { PasswordField } from "@/components/PasswordField";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { formatBytes } from "@/lib/format";

const CHECK_LABEL: Record<MailCheckResult["status"], string> = {
  ok: "Correct",
  missing: "Not published",
  different: "Different",
  error: "Lookup failed",
  pending: "Waiting",
};

const CHECK_TONE: Record<MailCheckResult["status"], string> = {
  ok: "text-success",
  missing: "text-warning",
  different: "text-destructive",
  error: "text-muted-foreground",
  pending: "text-muted-foreground",
};

/** One line for the whole check. Pending items (the DKIM key) are not a fault. */
function summary(results: MailCheckResult[], allOK: boolean): string {
  if (allOK) return "Every record is published and correct.";
  const faults = results.some(
    (r) => r.status === "missing" || r.status === "different" || r.status === "error",
  );
  if (faults)
    return "Some records are not in place yet. DNS changes can take up to a day to spread.";
  return "Almost there. The only open item is created when mail is turned on.";
}

const QUOTAS = [
  { mb: 1024, label: "1 GB" },
  { mb: 5120, label: "5 GB" },
  { mb: 10240, label: "10 GB" },
  { mb: 102400, label: "100 GB" },
];

/** One copyable DNS record, shown in the mail panel. */
function DNSRow(props: { type: string; name: string; value: string; note: string }) {
  const [copied, setCopied] = createSignal(false);
  const copy = async () => {
    await navigator.clipboard.writeText(props.value);
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };
  return (
    <tr class="border-t border-border align-top">
      <td class="px-3 py-2 font-mono text-xs">{props.type}</td>
      <td class="px-3 py-2 font-mono text-xs">{props.name}</td>
      <td class="px-3 py-2 font-mono text-xs break-all">
        {props.value || (
          <span class="text-muted-foreground">Turn mail on above to create this value</span>
        )}
      </td>
      <td class="px-3 py-2 text-xs text-muted-foreground">{props.note}</td>
      <td class="px-3 py-2">
        <Show when={props.value}>
          <Button
            variant="ghost"
            size="sm"
            aria-label={`Copy ${props.type} ${props.name} value`}
            onClick={() => void copy()}
          >
            {copied() ? <Check aria-hidden /> : <Copy aria-hidden />}
          </Button>
        </Show>
      </td>
    </tr>
  );
}

/** A mailbox address and its password, shown once after it is created or changed. */
interface Credentials {
  title: string;
  address: string;
  password: string;
}

function CredentialsCard(props: { credentials: Credentials; onDone: () => void }) {
  const [copied, setCopied] = createSignal(false);
  const copy = async () => {
    await navigator.clipboard.writeText(
      `${props.credentials.address}\n${props.credentials.password}`,
    );
    setCopied(true);
    window.setTimeout(() => setCopied(false), 1500);
  };
  return (
    <div
      role="status"
      class="space-y-2 rounded-md border border-success/40 bg-success/5 p-3 text-sm"
    >
      <p class="font-medium">{props.credentials.title}</p>
      <dl class="grid grid-cols-[auto_1fr] gap-x-3 gap-y-1">
        <dt class="text-muted-foreground">Address</dt>
        <dd class="break-all font-mono text-xs">{props.credentials.address}</dd>
        <dt class="text-muted-foreground">Password</dt>
        <dd class="break-all font-mono text-xs">{props.credentials.password}</dd>
      </dl>
      <p class="text-xs text-muted-foreground">
        Save this password now. It is not shown again, but you can set a new one at any time.
      </p>
      <div class="flex gap-2">
        <Button type="button" size="sm" variant="outline" onClick={() => void copy()}>
          {copied() ? <Check aria-hidden /> : <Copy aria-hidden />} Copy address and password
        </Button>
        <Button type="button" size="sm" variant="ghost" onClick={props.onDone}>
          Done
        </Button>
      </div>
    </div>
  );
}

/** Mail for one connected domain: the switch, mailboxes, and the DNS records to add. */
export function MailPanel(props: { domain: Domain }) {
  const mail = useMail(props.domain.id);
  const setMail = useSetMail(props.domain.id);
  const addBox = useAddMailbox(props.domain.id);
  const check = useCheckMailDNS(props.domain.id);
  const removeBox = useRemoveMailbox(props.domain.id);
  const changePassword = useChangeMailboxPassword(props.domain.id);
  const [credentials, setCredentials] = createSignal<Credentials | null>(null);
  const [changingId, setChangingId] = createSignal<string | null>(null);
  const [newPassword, setNewPassword] = createSignal("");
  const [localPart, setLocalPart] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [quota, setQuota] = createSignal(1024);

  const address = () => (localPart().trim() ? `${localPart().trim()}@${props.domain.apex}` : "");

  const submit = (e: SubmitEvent) => {
    e.preventDefault();
    addBox.mutate(
      { local_part: localPart().trim(), password: password(), quota_mb: quota() },
      {
        onSuccess: () => {
          setCredentials({
            title: "Mailbox created",
            address: address(),
            password: password(),
          });
          setLocalPart("");
          setPassword("");
        },
      },
    );
  };

  const savePassword = (id: string, boxAddress: string) => {
    changePassword.mutate(
      { id, password: newPassword() },
      {
        onSuccess: () => {
          setCredentials({
            title: "Password changed",
            address: boxAddress,
            password: newPassword(),
          });
          setChangingId(null);
          setNewPassword("");
        },
      },
    );
  };

  return (
    <div class="mt-3 space-y-5 rounded-md border border-border bg-muted/30 p-4">
      <Show when={mail.isError}>
        <Alert variant="destructive">
          <AlertDescription>{describeError(mail.error)}</AlertDescription>
        </Alert>
      </Show>

      <Show when={mail.data}>
        {(view) => (
          <>
            <Show when={view().webmail_url}>
              {(url) => (
                <div class="flex flex-wrap items-center justify-between gap-3 rounded-md border border-border bg-card px-3 py-2 text-sm">
                  <span>
                    Read and send mail in your browser. Sign in with the full mailbox address and
                    its password.
                  </span>
                  <a
                    href={url()}
                    target="_blank"
                    rel="noopener noreferrer"
                    class={buttonClasses("default", "sm")}
                  >
                    Open webmail
                  </a>
                </div>
              )}
            </Show>
            <div class="flex flex-wrap items-center justify-between gap-3">
              <label class="flex items-center gap-2 text-sm font-medium">
                <input
                  type="checkbox"
                  class="size-4 accent-primary"
                  checked={view().enabled}
                  disabled={setMail.isPending}
                  onChange={(e) => setMail.mutate(e.currentTarget.checked)}
                />
                Mail for {view().domain}
              </label>
              <Show when={!view().delivery_ready}>
                <span class="text-xs text-muted-foreground">
                  Mailboxes are saved now. Mail starts flowing once the mail server is set up.
                </span>
              </Show>
            </div>
            <Show when={!view().enabled}>
              <Alert>
                <AlertDescription>
                  Mail is off. Turn on the switch above to create the DKIM value and to add
                  mailboxes.
                </AlertDescription>
              </Alert>
            </Show>
            <Show when={setMail.error}>
              <Alert variant="destructive">
                <AlertDescription>{describeError(setMail.error)}</AlertDescription>
              </Alert>
            </Show>

            <section class="space-y-3">
              <h4 class="text-sm font-semibold">Mailboxes</h4>
              <Show
                when={view().mailboxes.length > 0}
                fallback={<p class="text-sm text-muted-foreground">No mailboxes yet.</p>}
              >
                <ul class="divide-y divide-border rounded-md border border-border bg-card">
                  <For each={view().mailboxes}>
                    {(box) => (
                      <li class="flex flex-wrap items-center gap-3 px-3 py-2 text-sm">
                        <span class="font-medium break-all">
                          {box.local_part}@{view().domain}
                        </span>
                        <Show when={changingId() === box.id}>
                          <form
                            class="flex w-full flex-wrap items-end gap-2"
                            onSubmit={(e) => {
                              e.preventDefault();
                              savePassword(box.id, `${box.local_part}@${view().domain}`);
                            }}
                          >
                            <div class="min-w-64 flex-1">
                              <PasswordField
                                id={`mb-new-${box.id}`}
                                label="New password (12+ characters)"
                                value={newPassword()}
                                onChange={setNewPassword}
                              />
                            </div>
                            <Button
                              type="submit"
                              size="sm"
                              disabled={changePassword.isPending || newPassword().length < 12}
                            >
                              {changePassword.isPending ? "Saving…" : "Save password"}
                            </Button>
                            <Button
                              type="button"
                              size="sm"
                              variant="ghost"
                              onClick={() => {
                                setChangingId(null);
                                setNewPassword("");
                              }}
                            >
                              Cancel
                            </Button>
                          </form>
                          <Show when={changePassword.error}>
                            <p class="w-full text-xs text-destructive" role="alert">
                              {describeError(changePassword.error)}
                            </p>
                          </Show>
                        </Show>
                        <span class="text-xs text-muted-foreground">
                          {formatBytes(box.quota_mb * 1024 * 1024)} quota
                        </span>
                        <Show when={!box.active}>
                          <span class="text-xs text-warning">disabled</span>
                        </Show>
                        <Show when={changingId() !== box.id}>
                          <Button
                            variant="outline"
                            size="sm"
                            class="ml-auto"
                            aria-label={`Change password of ${box.local_part}@${view().domain}`}
                            onClick={() => {
                              setChangingId(box.id);
                              setNewPassword("");
                              changePassword.reset();
                            }}
                          >
                            Change password
                          </Button>
                        </Show>
                        <Button
                          variant="ghost"
                          size="sm"
                          class={
                            changingId() !== box.id
                              ? "text-destructive"
                              : "ml-auto text-destructive"
                          }
                          aria-label={`Remove ${box.local_part}@${view().domain}`}
                          disabled={removeBox.isPending}
                          onClick={() => removeBox.mutate(box.id)}
                        >
                          <Trash2 aria-hidden /> Remove
                        </Button>
                      </li>
                    )}
                  </For>
                </ul>
              </Show>
              <Show when={removeBox.error}>
                <Alert variant="destructive">
                  <AlertDescription>{describeError(removeBox.error)}</AlertDescription>
                </Alert>
              </Show>

              <form
                class="grid gap-3 sm:grid-cols-[1fr_1fr_auto_auto] sm:items-end"
                onSubmit={submit}
                noValidate
              >
                <div class="space-y-1.5">
                  <Label for={`mb-local-${props.domain.id}`}>Name before @</Label>
                  <div class="flex items-center gap-1">
                    <Input
                      id={`mb-local-${props.domain.id}`}
                      placeholder="info"
                      autocomplete="off"
                      value={localPart()}
                      onInput={(e) => setLocalPart(e.currentTarget.value.toLowerCase())}
                    />
                    <span class="whitespace-nowrap text-sm text-muted-foreground">
                      @{props.domain.apex}
                    </span>
                  </div>
                </div>
                <PasswordField
                  id={`mb-pass-${props.domain.id}`}
                  label="Password (12+ characters)"
                  value={password()}
                  onChange={setPassword}
                />
                <div class="space-y-1.5">
                  <Label for={`mb-quota-${props.domain.id}`}>Quota</Label>
                  <select
                    id={`mb-quota-${props.domain.id}`}
                    class="h-9 rounded-md border border-input bg-background px-2 text-sm"
                    value={quota()}
                    onChange={(e) => setQuota(Number(e.currentTarget.value))}
                  >
                    <For each={QUOTAS}>{(q) => <option value={q.mb}>{q.label}</option>}</For>
                  </select>
                </div>
                <Button
                  type="submit"
                  disabled={
                    !view().enabled || addBox.isPending || !address() || password().length < 12
                  }
                >
                  {addBox.isPending ? "Adding…" : "Add mailbox"}
                </Button>
              </form>
              <Show when={addBox.error}>
                <Alert variant="destructive">
                  <AlertDescription>{describeError(addBox.error)}</AlertDescription>
                </Alert>
              </Show>
              <Show when={credentials()}>
                {(c) => <CredentialsCard credentials={c()} onDone={() => setCredentials(null)} />}
              </Show>
              <Show when={!view().enabled}>
                <p class="text-xs text-muted-foreground">
                  Turn mail on above before adding mailboxes.
                </p>
              </Show>
            </section>

            <section class="space-y-2">
              <div class="flex flex-wrap items-center justify-between gap-2">
                <h4 class="text-sm font-semibold">DNS records to add at your DNS provider</h4>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={check.isPending}
                  onClick={() => check.mutate(undefined)}
                >
                  {check.isPending ? "Checking…" : "Check DNS now"}
                </Button>
              </div>
              <Show when={check.error}>
                <Alert variant="destructive">
                  <AlertDescription>{describeError(check.error)}</AlertDescription>
                </Alert>
              </Show>
              <Show when={check.data}>
                {(report) => (
                  <div class="space-y-2 rounded-md border border-border bg-card p-3 text-sm">
                    <p class={report().all_ok ? "font-medium text-success" : "font-medium"}>
                      {summary(report().results, report().all_ok)}
                    </p>
                    <ul class="space-y-1">
                      <For each={report().results}>
                        {(r) => (
                          <li class="flex flex-wrap gap-2">
                            <span class="font-mono text-xs">
                              {r.type} {r.name}
                            </span>
                            <span class={`text-xs font-medium ${CHECK_TONE[r.status]}`}>
                              {CHECK_LABEL[r.status]}
                            </span>
                            <span class="text-xs text-muted-foreground">{r.message}</span>
                          </li>
                        )}
                      </For>
                    </ul>
                  </div>
                )}
              </Show>
              <div class="overflow-x-auto rounded-md border border-border bg-card">
                <table class="w-full text-left text-sm">
                  <thead class="bg-muted/50 text-xs uppercase tracking-wide text-muted-foreground">
                    <tr>
                      <th class="px-3 py-2 font-medium">Type</th>
                      <th class="px-3 py-2 font-medium">Name</th>
                      <th class="px-3 py-2 font-medium">Value</th>
                      <th class="px-3 py-2 font-medium">Why</th>
                      <th class="px-3 py-2" />
                    </tr>
                  </thead>
                  <tbody>
                    <For each={view().dns}>
                      {(r) => <DNSRow type={r.type} name={r.name} value={r.value} note={r.note} />}
                    </For>
                  </tbody>
                </table>
              </div>
            </section>
          </>
        )}
      </Show>
    </div>
  );
}
