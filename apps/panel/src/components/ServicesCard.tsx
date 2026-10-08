import { useServices } from "@/api/queries";
import type { ServiceState } from "@/api/types";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { cn } from "@/lib/utils";

const LABELS: Record<string, string> = {
  postgresql: "PostgreSQL",
  mariadb: "MariaDB",
  "redis-server": "Redis",
  caddy: "Caddy (web server)",
  fail2ban: "Fail2Ban",
  "zenex-api": "Panel API",
  "zenex-helper": "Website helper",
};

function label(name: string): string {
  if (LABELS[name]) return LABELS[name];
  const php = /^php(.+)-fpm$/.exec(name);
  if (php) return `PHP ${php[1]} (FPM)`;
  return name;
}

function tone(state: string): { dot: string; text: string } {
  if (state === "active") return { dot: "bg-success", text: "Running" };
  if (state === "failed") return { dot: "bg-destructive", text: "Failed" };
  if (state === "inactive") return { dot: "bg-muted-foreground", text: "Stopped" };
  return { dot: "bg-warning", text: state };
}

function ServiceRow({ service }: { service: ServiceState }) {
  const t = tone(service.state);
  return (
    <li className="flex items-center gap-3 py-2.5">
      <span className={cn("size-2 shrink-0 rounded-full", t.dot)} aria-hidden />
      <span className="min-w-0 flex-1 text-sm font-medium">{label(service.name)}</span>
      <span className="shrink-0 text-xs text-muted-foreground">{t.text}</span>
    </li>
  );
}

/** Live state of every service the panel depends on. */
export function ServicesCard() {
  const services = useServices();
  const problems = services.data?.filter((s) => s.state !== "active").length ?? 0;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Services</CardTitle>
        <CardDescription>
          {services.data
            ? problems === 0
              ? "Everything is running."
              : `${problems} service${problems === 1 ? "" : "s"} need attention.`
            : "Checking services…"}
        </CardDescription>
      </CardHeader>
      <CardContent>
        {services.isError && (
          <Alert variant="destructive">
            <AlertDescription>Could not check services right now.</AlertDescription>
          </Alert>
        )}
        {services.data && (
          <ul className="divide-y divide-border">
            {services.data.map((s) => (
              <ServiceRow key={s.name} service={s} />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
