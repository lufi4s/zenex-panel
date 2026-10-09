import { createSignal, For, Show, type JSX } from "solid-js";
import { useBranding, useLogin } from "@/api/queries";
import { ApiError, describeError } from "@/api/client";
import { Activity, Eye, EyeOff, Globe, Loader2, ShieldCheck } from "@/components/icons";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { brandAssetVersion } from "@/lib/brand";
import { cn } from "@/lib/utils";

const FEATURES: { icon: typeof Globe; title: string; text: string }[] = [
  {
    icon: Globe,
    title: "WordPress in minutes",
    text: "Create a site on your domain without touching a server.",
  },
  {
    icon: ShieldCheck,
    title: "HTTPS handled for you",
    text: "Certificates are issued and renewed automatically.",
  },
  {
    icon: Activity,
    title: "Watch everything",
    text: "Server health, uptime, logs and activity in one place.",
  },
];

/** The saved logo, or the first letter of the panel name when there is none. */
function Mark(props: { class?: string; name: string; hasLogo?: boolean }) {
  return (
    <span
      aria-hidden
      class={cn(
        "flex size-10 items-center justify-center overflow-hidden rounded-xl bg-primary text-lg font-bold text-primary-foreground shadow-sm",
        props.class,
      )}
    >
      <Show when={props.hasLogo} fallback={props.name.trim().charAt(0).toUpperCase() || "Z"}>
        <img
          src={`/api/v1/branding/logo?v=${brandAssetVersion()}`}
          alt=""
          class="size-full object-contain"
        />
      </Show>
    </span>
  );
}

/** Two-column on wide screens: the brand on the left, the form on the right. */
function Frame(props: { children: JSX.Element; name: string; tagline: string; hasLogo?: boolean }) {
  return (
    <div class="grid min-h-dvh bg-background lg:grid-cols-[minmax(0,1.05fr)_minmax(0,1fr)]">
      <aside class="relative hidden flex-col justify-between overflow-hidden bg-primary p-12 text-primary-foreground lg:flex">
        <div class="relative flex items-center gap-3">
          <Mark
            class="bg-primary-foreground text-primary"
            name={props.name}
            hasLogo={props.hasLogo}
          />
          <span class="text-lg font-semibold tracking-tight">{props.name}</span>
        </div>
        <div class="relative max-w-md space-y-8">
          <div class="space-y-3">
            <h1 class="text-4xl font-semibold leading-tight">{props.tagline}</h1>
            <p class="text-base opacity-80">
              One panel for your domains, websites, servers and team.
            </p>
          </div>
          <ul class="space-y-5">
            <For each={FEATURES}>
              {(f) => (
                <li class="flex gap-4">
                  <span class="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary-foreground/15">
                    <f.icon class="size-4" aria-hidden />
                  </span>
                  <div>
                    <p class="font-medium">{f.title}</p>
                    <p class="text-sm opacity-75">{f.text}</p>
                  </div>
                </li>
              )}
            </For>
          </ul>
        </div>
        <p class="relative text-xs opacity-60">
          © {new Date().getFullYear()} {props.name}
        </p>
      </aside>

      <main class="flex flex-col justify-center px-5 py-10 sm:px-10">
        <div class="mx-auto w-full max-w-sm">
          <div class="mb-8 flex items-center gap-3 lg:hidden">
            <Mark name={props.name} hasLogo={props.hasLogo} />
            <div>
              <p class="font-semibold leading-tight">{props.name}</p>
              <p class="text-xs text-muted-foreground">{props.tagline}</p>
            </div>
          </div>
          {props.children}
        </div>
      </main>
    </div>
  );
}

export function LoginPage() {
  const branding = useBranding();
  const login = useLogin();
  const [email, setEmail] = createSignal("");
  const [password, setPassword] = createSignal("");
  const [showPassword, setShowPassword] = createSignal(false);

  const name = () => branding.data?.name ?? "Zenex Panel";
  const tagline = () => branding.data?.tagline ?? "Manage your WordPress websites";
  const message = () =>
    login.isError
      ? login.error instanceof ApiError
        ? login.error.message
        : describeError(login.error)
      : null;
  const ready = () => email().trim() !== "" && password() !== "";

  const submit = (event: SubmitEvent) => {
    event.preventDefault();
    if (!ready() || login.isPending) return;
    login.mutate({ email: email().trim(), password: password() });
  };

  return (
    <Frame name={name()} tagline={tagline()} hasLogo={branding.data?.has_logo}>
      <div class="space-y-1.5">
        <h2 class="text-2xl font-semibold tracking-tight">Welcome back</h2>
        <p class="text-sm text-muted-foreground">Sign in to continue to your dashboard.</p>
      </div>

      <form onSubmit={submit} class="mt-8 space-y-5" noValidate>
        <div class="space-y-2">
          <Label for="email">Email</Label>
          <Input
            id="email"
            type="email"
            autocomplete="username"
            inputmode="email"
            autocapitalize="none"
            required
            maxLength={254}
            value={email()}
            onInput={(e) => setEmail(e.currentTarget.value)}
            placeholder="you@company.com"
          />
        </div>

        <div class="space-y-2">
          <div class="flex items-center justify-between">
            <Label for="password">Password</Label>
          </div>
          <div class="relative">
            <Input
              id="password"
              type={showPassword() ? "text" : "password"}
              autocomplete="current-password"
              required
              maxLength={1024}
              value={password()}
              onInput={(e) => setPassword(e.currentTarget.value)}
              class="pr-11"
            />
            <button
              type="button"
              onClick={() => setShowPassword((v) => !v)}
              aria-label={showPassword() ? "Hide password" : "Show password"}
              aria-pressed={showPassword()}
              class="absolute right-1 top-1/2 flex size-9 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground hover:text-foreground"
            >
              <Show when={showPassword()} fallback={<Eye class="size-4" aria-hidden />}>
                <EyeOff class="size-4" aria-hidden />
              </Show>
            </button>
          </div>
        </div>

        <Show when={message()}>
          <Alert variant="destructive">
            <AlertDescription>{message()}</AlertDescription>
          </Alert>
        </Show>

        <Button
          type="submit"
          size="lg"
          class="w-full text-base"
          disabled={!ready() || login.isPending}
        >
          <Show when={login.isPending} fallback={"Sign in"}>
            <Loader2 class="animate-spin" aria-hidden /> Signing in…
          </Show>
        </Button>
      </form>

      <p class="mt-8 text-center text-xs text-muted-foreground">
        Trouble signing in? Ask the administrator of this server to reset your password.
      </p>
    </Frame>
  );
}
