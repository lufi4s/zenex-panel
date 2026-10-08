import { useServices } from "@/api/queries";
import type { ServiceState } from "@/api/types";
import { Alert } from "@/components/ui/badge-alert";
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

function ServiceTile({ service }: { service: ServiceState }) {
  const t = tone(service.state);
  return (
    <li className="flex items-center gap-3 rounded-md border border-border p-3">
      <span className={cn("size-2.5 shrink-0 rounded-full", t.dot)} aria-hidden />
      <div className="min-w-0">
        <div className="truncate text-sm font-medium">{label(service.name)}</div>
        <div className="text-xs text-muted-foreground">{t.text}</div>
      </div>
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
        {services.isError && <Alert tone="danger">Could not check services right now.</Alert>}
        {services.data && (
          <ul className="grid grid-cols-1 gap-2 sm:grid-cols-2 lg:grid-cols-3">
            {services.data.map((s) => (
              <ServiceTile key={s.name} service={s} />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
