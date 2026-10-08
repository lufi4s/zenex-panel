import { useState, type FormEvent } from "react";
import { useLogin } from "@/api/queries";
import { ApiError } from "@/api/client";
import { Alert } from "@/components/ui/badge-alert";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Input, Label } from "@/components/ui/input";

export function LoginPage() {
  const login = useLogin();
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");

  const submit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    login.mutate({ email: email.trim(), password });
  };

  const message =
    login.error instanceof ApiError
      ? login.error.message
      : login.error
        ? "Sign-in failed. Try again."
        : null;

  return (
    <main className="flex min-h-dvh items-center justify-center p-4">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <CardTitle>Sign in to Zenex</CardTitle>
          <CardDescription>Manage your websites from one place.</CardDescription>
        </CardHeader>
        <CardContent>
          <form onSubmit={submit} className="space-y-4" noValidate>
            <div className="space-y-1.5">
              <Label htmlFor="email">Email</Label>
              <Input
                id="email"
                type="email"
                autoComplete="username"
                required
                maxLength={254}
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
                maxLength={1024}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </div>
            {message && <Alert tone="danger">{message}</Alert>}
            <Button
              type="submit"
              className="w-full"
              disabled={login.isPending || !email || !password}
            >
              {login.isPending ? "Signing in…" : "Sign in"}
            </Button>
          </form>
        </CardContent>
      </Card>
    </main>
  );
}
