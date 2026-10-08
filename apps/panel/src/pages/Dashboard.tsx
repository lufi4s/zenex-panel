import { lazy, Suspense, useState, type ReactNode } from "react";
import type { User } from "@/api/types";
import { ActivityCard } from "@/components/ActivityCard";
import { AppShell } from "@/components/AppShell";
import { BrandingCard } from "@/components/BrandingCard";
import { DomainsCard } from "@/components/DomainsCard";
import { JobProgress } from "@/components/JobProgress";
import { NewWebsiteCard } from "@/components/NewWebsiteCard";
import { ServerStrip } from "@/components/ServerStrip";
import { WebsitesCard } from "@/components/WebsitesCard";
import { CardSkeleton } from "@/components/ui/skeleton";

// Heavier sections load on demand, so the first paint stays quick.
const MonitoringCard = lazy(() =>
  import("@/components/MonitoringCard").then((m) => ({ default: m.MonitoringCard })),
);
const ServicesCard = lazy(() =>
  import("@/components/ServicesCard").then((m) => ({ default: m.ServicesCard })),
);

interface ActiveJob {
  jobId: string;
  title: string;
}

/**
 * A page section is only an anchor: each card inside carries its own title, so
 * nothing is labelled twice.
 */
function Section({ id, children }: { id: string; children: ReactNode }) {
  return (
    <section id={id} className="scroll-mt-28">
      {children}
    </section>
  );
}

/** Page title shared by the top of the dashboard. */
function PageHeader({ greeting }: { greeting: string }) {
  return (
    <div className="space-y-1">
      <h1 className="font-heading text-2xl font-semibold tracking-tight sm:text-3xl">Overview</h1>
      <p className="text-sm text-muted-foreground">{greeting}</p>
    </div>
  );
}

/** The panel on one scrollable page, arranged on a 12-column grid. */
export function Dashboard({ user }: { user: User }) {
  const [activeJob, setActiveJob] = useState<ActiveJob | null>(null);
  const startJob = (jobId: string, title: string) => setActiveJob({ jobId, title });
  const isAdmin = user.roles.includes("administrator");
  const firstName = user.email.split("@")[0] ?? "there";

  return (
    <AppShell user={user}>
      <PageHeader
        greeting={`Welcome back, ${firstName}. Here is how your server and websites are doing.`}
      />

      <Section id="overview">
        <ServerStrip />
      </Section>

      <div className="grid gap-6 lg:grid-cols-12">
        <div className="lg:col-span-8">
          <Suspense fallback={<CardSkeleton height="h-80" />}>
            <Section id="monitoring">
              <MonitoringCard />
            </Section>
          </Suspense>
        </div>
        <div className="lg:col-span-4">
          <Suspense fallback={<CardSkeleton height="h-64" />}>
            <Section id="services">
              <ServicesCard />
            </Section>
          </Suspense>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-12">
        <div className="lg:col-span-5">
          <Section id="domains">
            <DomainsCard />
          </Section>
        </div>
        <div className="lg:col-span-7">
          <Section id="new-website">
            <NewWebsiteCard onStarted={startJob} />
          </Section>
        </div>
      </div>

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

      {isAdmin && (
        <Section id="branding">
          <BrandingCard />
        </Section>
      )}
    </AppShell>
  );
}
