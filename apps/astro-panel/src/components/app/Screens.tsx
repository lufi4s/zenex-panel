import { AppShell } from "@/components/app/AppShell";
import { ComingSoon } from "@/components/app/ComingSoon";
import { Login } from "@/components/app/Login";
import { Overview } from "@/components/app/Overview";
import { Providers } from "@/components/app/Providers";

/** Each page of the site is one React island that brings its own providers and frame. */
export function LoginScreen() {
  return (
    <Providers>
      <Login />
    </Providers>
  );
}

export function OverviewScreen() {
  return (
    <Providers>
      <AppShell active="Overview" title="Overview">
        <Overview />
      </AppShell>
    </Providers>
  );
}

export function SectionScreen({ active }: { active: string }) {
  return (
    <Providers>
      <AppShell active={active} title={active}>
        <ComingSoon title={active} />
      </AppShell>
    </Providers>
  );
}
