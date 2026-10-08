import { useState, type ReactNode } from "react";
import { LogOut } from "lucide-react";
import { useLogout } from "@/api/queries";
import type { User } from "@/api/types";
import { ActivityCard } from "@/components/ActivityCard";
import { DomainsCard } from "@/components/DomainsCard";
import { JobProgress } from "@/components/JobProgress";
import { MonitoringCard } from "@/components/MonitoringCard";
import { NewWebsiteCard } from "@/components/NewWebsiteCard";
import { ServerStrip } from "@/components/ServerStrip";
import { ServicesCard } from "@/components/ServicesCard";
import { WebsitesCard } from "@/components/WebsitesCard";
import { Button } from "@/components/ui/button";

interface ActiveJob {
  jobId: string;
  title: string;
}

const SECTIONS = [
  { id: "monitoring", label: "Monitoring" },
  { id: "services", label: "Services" },
  { id: "domains", label: "Domains" },
  { id: "websites", label: "Websites" },
  { id: "activity", label: "Activity" },
] as const;

/** A page section that scrolls below the sticky header when opened from the menu. */
function Section({ id, children }: { id: string; children: ReactNode }) {
  return (
    <section id={id} className="scroll-mt-20">
      {children}
    </section>
  );
}

/**
 * The whole panel on one scrollable page. The menu jumps to a section; nothing
 * is hidden behind tabs.
 */
export function Dashboard({ user }: { user: User }) {
  const logout = useLogout();
  const [activeJob, setActiveJob] = useState<ActiveJob | null>(null);
  const startJob = (jobId: string, title: string) => setActiveJob({ jobId, title });

  return (
    <div className="min-h-dvh">
      <header className="sticky top-0 z-20 border-b border-border bg-card/90 backdrop-blur">
        <div className="mx-auto flex max-w-5xl items-center gap-4 px-4 py-3">
          <span className="font-semibold tracking-tight">Zenex</span>
          <nav aria-label="Sections" className="hidden items-center gap-1 md:flex">
            {SECTIONS.map((s) => (
              <a
                key={s.id}
                href={`#${s.id}`}
                className="rounded-md px-2.5 py-1 text-sm text-muted-foreground hover:bg-muted hover:text-foreground"
              >
                {s.label}
              </a>
            ))}
          </nav>
          <span className="ml-auto truncate text-sm text-muted-foreground">{user.email}</span>
          <Button
            variant="ghost"
            size="sm"
            disabled={logout.isPending}
            onClick={() => logout.mutate()}
          >
            <LogOut aria-hidden />
            <span className="hidden sm:inline">Sign out</span>
          </Button>
        </div>
      </header>

      <main className="mx-auto flex max-w-5xl flex-col gap-6 px-4 py-6">
        <ServerStrip />
        <Section id="monitoring">
          <MonitoringCard />
        </Section>
        <Section id="services">
          <ServicesCard />
        </Section>
        <Section id="domains">
          <DomainsCard />
        </Section>
        <NewWebsiteCard onStarted={startJob} />
        {activeJob && (
          <JobProgress
            key={activeJob.jobId}
            jobId={activeJob.jobId}
            title={activeJob.title}
            onDismiss={() => setActiveJob(null)}
          />
        )}
        <Section id="websites">
          <WebsitesCard onStarted={startJob} />
        </Section>
        <Section id="activity">
          <ActivityCard />
        </Section>
      </main>
    </div>
  );
}
