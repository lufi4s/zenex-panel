import { useState, type FormEvent, type ReactNode } from "react";
import { useBranding, useLogin } from "@/api/queries";
import { ApiError, describeError } from "@/api/client";
import { Activity, Eye, EyeOff, Globe, Loader2, ShieldCheck } from "@/components/icons";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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

function Mark({ className }: { className?: string }) {
  return (
    <span
      aria-hidden
      className={cn(
        "flex size-10 items-center justify-center rounded-xl bg-primary text-lg font-bold text-primary-foreground shadow-sm",
        className,
      )}
    >
      Z
    </span>
  );
}

/** Two-column on wide screens: the brand on the left, the form on the right. */
function Frame({
  children,
  name,
  tagline,
}: {
  children: ReactNode;
  name: string;
  tagline: string;
}) {
  return (
    <div className="grid min-h-dvh bg-background lg:grid-cols-[minmax(0,1.05fr)_minmax(0,1fr)]">
      <aside className="relative hidden flex-col justify-between overflow-hidden bg-primary p-12 text-primary-foreground lg:flex">
        <div className="relative flex items-center gap-3">
          <Mark className="bg-primary-foreground text-primary" />
          <span className="text-lg font-semibold tracking-tight">{name}</span>
        </div>
        <div className="relative max-w-md space-y-8">
          <div className="space-y-3">
            <h1 className="text-4xl font-semibold leading-tight">{tagline}</h1>
            <p className="text-base opacity-80">
              One panel for your domains, websites, servers and team.
            </p>
          </div>
          <ul className="space-y-5">
            {FEATURES.map((f) => (
              <li key={f.title} className="flex gap-4">
                <span className="flex size-9 shrink-0 items-center justify-center rounded-lg bg-primary-foreground/15">
                  <f.icon className="size-4" aria-hidden />
                </span>
                <div>
                  <p className="font-medium">{f.title}</p>
                  <p className="text-sm opacity-75">{f.text}</p>
                </div>
              </li>
            ))}
          </ul>
        </div>
        <p className="relative text-xs opacity-60">
          © {new Date().getFullYear()} {name}
        </p>
      </aside>

      <main className="flex flex-col justify-center px-5 py-10 sm:px-10">
        <div className="mx-auto w-full max-w-sm">
          <div className="mb-8 flex items-center gap-3 lg:hidden">
            <Mark />
            <div>
              <p className="font-semibold leading-tight">{name}</p>
              <p className="text-xs text-muted-foreground">{tagline}</p>
            </div>
          </div>
          {children}
        </div>
      </main>
    </div>
  );
}

export function LoginPage() {
  const branding = useBranding();
  const login = useLogin();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);

  const name = branding.data?.name ?? "Zenex Panel";
  const tagline = branding.data?.tagline ?? "Manage your WordPress websites";
  const message = login.isError
    ? login.error instanceof ApiError
      ? login.error.message
      : describeError(login.error)
    : null;
  const ready = email.trim() !== "" && password !== "";

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!ready || login.isPending) return;
    login.mutate({ email: email.trim(), password });
  };

  return (
    <Frame name={name} tagline={tagline}>
      <div className="space-y-1.5">
        <h2 className="text-2xl font-semibold tracking-tight">Welcome back</h2>
        <p className="text-sm text-muted-foreground">Sign in to continue to your dashboard.</p>
      </div>

      <form onSubmit={submit} className="mt-8 space-y-5" noValidate>
        <div className="space-y-2">
          <Label htmlFor="email">Email</Label>
          <Input
            id="email"
            type="email"
            autoComplete="username"
            inputMode="email"
            autoCapitalize="none"
            required
            maxLength={254}
            value={email}
            onChange={(e) => setEmail(e.target.value)}
            placeholder="you@company.com"
          />
        </div>

        <div className="space-y-2">
          <div className="flex items-center justify-between">
            <Label htmlFor="password">Password</Label>
          </div>
          <div className="relative">
            <Input
              id="password"
              type={showPassword ? "text" : "password"}
              autoComplete="current-password"
              required
              maxLength={1024}
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              className="pr-11"
            />
            <button
              type="button"
              onClick={() => setShowPassword((v) => !v)}
              aria-label={showPassword ? "Hide password" : "Show password"}
              aria-pressed={showPassword}
              className="absolute right-1 top-1/2 flex size-9 -translate-y-1/2 items-center justify-center rounded-md text-muted-foreground hover:text-foreground"
            >
              {showPassword ? (
                <EyeOff className="size-4" aria-hidden />
              ) : (
                <Eye className="size-4" aria-hidden />
              )}
            </button>
          </div>
        </div>

        {message && (
          <Alert variant="destructive">
            <AlertDescription>{message}</AlertDescription>
          </Alert>
        )}

        <Button
          type="submit"
          size="lg"
          className="w-full text-base"
          disabled={!ready || login.isPending}
        >
          {login.isPending ? (
            <>
              <Loader2 className="animate-spin" aria-hidden /> Signing in…
            </>
          ) : (
            "Sign in"
          )}
        </Button>
      </form>

      <p className="mt-8 text-center text-xs text-muted-foreground">
        Trouble signing in? Ask the administrator of this server to reset your password.
      </p>
    </Frame>
  );
}
