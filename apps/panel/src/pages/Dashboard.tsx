import { useState } from "react";
import { LogOut } from "lucide-react";
import { useLogout } from "@/api/queries";
import type { User } from "@/api/types";
import { JobProgress } from "@/components/JobProgress";
import { DomainsCard } from "@/components/DomainsCard";
import { NewWebsiteCard } from "@/components/NewWebsiteCard";
import { ServerStrip } from "@/components/ServerStrip";
import { WebsitesCard } from "@/components/WebsitesCard";
import { Button } from "@/components/ui/button";

interface ActiveJob {
  jobId: string;
  title: string;
}

/** The whole panel on one page: server, domain, new website, then your websites. */
export function Dashboard({ user }: { user: User }) {
  const logout = useLogout();
  const [activeJob, setActiveJob] = useState<ActiveJob | null>(null);

  return (
    <div className="min-h-dvh">
      <header className="sticky top-0 z-10 border-b border-border bg-card/90 backdrop-blur">
        <div className="mx-auto flex max-w-4xl items-center gap-3 px-4 py-3">
          <span className="font-semibold tracking-tight">Zenex</span>
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

      <main className="mx-auto flex max-w-4xl flex-col gap-6 px-4 py-6">
        <ServerStrip />
        <DomainsCard />
        <NewWebsiteCard onStarted={(jobId, title) => setActiveJob({ jobId, title })} />
        {activeJob && (
          <JobProgress
            key={activeJob.jobId}
            jobId={activeJob.jobId}
            title={activeJob.title}
            onDismiss={() => setActiveJob(null)}
          />
        )}
        <WebsitesCard onStarted={(jobId, title) => setActiveJob({ jobId, title })} />
      </main>
    </div>
  );
}
