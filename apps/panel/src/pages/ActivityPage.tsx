import { ActivityCard } from "@/components/ActivityCard";
import { PageHeader } from "@/components/PageHeader";

/** Audit trail of everything done in the panel. */
export function ActivityPage() {
  return (
    <>
      <PageHeader
        title="Activity"
        description="Everything that happened on this server, newest first."
      />
      <ActivityCard />
    </>
  );
}
