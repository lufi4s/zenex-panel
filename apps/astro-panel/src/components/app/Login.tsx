import { useState, type SyntheticEvent } from "react";
import { Loader2 } from "lucide-react";
import { ApiError } from "@/lib/api";
import { useLogin } from "@/lib/queries";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";

/** Sign-in page: a brand panel on the left, the form on the right. */
export function Login() {
  const login = useLogin();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const submit = (e: SyntheticEvent<HTMLFormElement>) => {
    e.preventDefault();
    login.mutate({ email, password }, { onSuccess: () => window.location.assign("/") });
  };

  const error =
    login.error instanceof ApiError
      ? login.error.message
      : login.isError
        ? "Could not sign in. Please try again."
        : null;

  return (
    <div className="grid min-h-svh lg:grid-cols-2">
      <div className="relative hidden overflow-hidden border-r bg-card lg:flex lg:flex-col lg:justify-between lg:p-10">
        <div className="pointer-events-none absolute inset-0 bg-[radial-gradient(70%_50%_at_30%_20%,oklch(0.7_0.18_275/25%),transparent)]" />
        <div className="relative flex items-center gap-2">
          <div className="flex size-9 items-center justify-center rounded-lg bg-primary text-sm font-semibold text-primary-foreground">
            Z
          </div>
          <span className="font-semibold">Zenex Panel</span>
        </div>
        <div className="relative max-w-md space-y-3">
          <p className="text-3xl font-semibold leading-tight">Every WordPress site, on one calm dashboard.</p>
          <p className="text-muted-foreground">
            Websites, domains, mail, backups and the server, all managed from one place.
          </p>
        </div>
      </div>

      <div className="flex items-center justify-center p-6">
        <form onSubmit={submit} className="w-full max-w-sm space-y-6">
          <div className="space-y-1.5">
            <h2 className="text-2xl font-semibold">Sign in</h2>
            <p className="text-sm text-muted-foreground">Use your panel administrator account.</p>
          </div>

          {error && (
            <Alert variant="destructive">
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          )}

          <div className="space-y-4">
            <div className="space-y-1.5">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                autoComplete="username"
                required
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor="password">Password</Label>
              <Input
                id="password"
                type="password"
                autoComplete="current-password"
                required
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
          </div>

          <Button type="submit" className="w-full" disabled={login.isPending}>
            {login.isPending && <Loader2 className="animate-spin" />}
            {login.isPending ? "Signing in" : "Sign in"}
          </Button>
        </form>
      </div>
    </div>
  );
}
