import { lazy, Suspense, useState, type ReactNode } from "react";
import type { User } from "@/api/types";
import { ActivityCard } from "@/components/ActivityCard";
import { BrandingCard } from "@/components/BrandingCard";
import { AppShell } from "@/components/AppShell";
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

/** A page section. Its id is the target of the matching sidebar link. */
function Section({
  id,
  title,
  description,
  children,
}: {
  id: string;
  title: string;
  description?: string;
  children: ReactNode;
}) {
  return (
    <section id={id} className="scroll-mt-20 space-y-4">
      <div>
        <h2 className="text-lg font-semibold tracking-tight">{title}</h2>
        {description && <p className="text-sm text-muted-foreground">{description}</p>}
      </div>
      {children}
    </section>
  );
}

/** The whole panel on one scrollable page, arranged as a dashboard grid. */
export function Dashboard({ user }: { user: User }) {
  const [activeJob, setActiveJob] = useState<ActiveJob | null>(null);
  const startJob = (jobId: string, title: string) => setActiveJob({ jobId, title });

  return (
    <AppShell user={user}>
      <Section id="overview" title="Overview" description="This server right now.">
        <ServerStrip />
      </Section>

      <div className="grid gap-6 xl:grid-cols-3">
        <div className="xl:col-span-2">
          <Suspense fallback={<CardSkeleton height="h-80" />}>
            <Section
              id="monitoring"
              title="Server history"
              description="CPU, memory and disk over time."
            >
              <MonitoringCard />
            </Section>
          </Suspense>
        </div>
        <div>
          <Suspense fallback={<CardSkeleton height="h-64" />}>
            <Section id="services" title="Services" description="Everything the panel depends on.">
              <ServicesCard />
            </Section>
          </Suspense>
        </div>
      </div>

      <div className="grid gap-6 lg:grid-cols-2">
        <Section id="domains" title="Domains" description="Where your websites live.">
          <DomainsCard />
        </Section>
        <Section
          id="new-website"
          title="New website"
          description="Build a WordPress site in a few minutes."
        >
          <NewWebsiteCard onStarted={startJob} />
        </Section>
      </div>

      {activeJob && (
        <JobProgress
          key={activeJob.jobId}
          jobId={activeJob.jobId}
          title={activeJob.title}
          onDismiss={() => setActiveJob(null)}
        />
      )}

      <Section
        id="websites"
        title="Websites"
        description="Open, manage, check and delete your sites."
      >
        <WebsitesCard onStarted={startJob} />
      </Section>

      {user.roles.includes("administrator") && (
        <Section
          id="branding"
          title="Branding"
          description="Name, tagline and colour of this panel."
        >
          <BrandingCard />
        </Section>
      )}

      <Section
        id="activity"
        title="Activity"
        description="Everything that happened on your account."
      >
        <ActivityCard />
      </Section>
    </AppShell>
  );
}
