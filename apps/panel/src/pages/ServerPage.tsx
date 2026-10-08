import { MonitoringCard } from "@/components/MonitoringCard";
import { PageHeader } from "@/components/PageHeader";
import { ServerStrip } from "@/components/ServerStrip";
import { ServicesCard } from "@/components/ServicesCard";

/** Server health: resource usage over time, and the status of each service. */
export function ServerPage() {
  return (
    <>
      <PageHeader
        title="Server"
        description="CPU, memory, disk and network over time, plus the services that run this panel."
      />
      <ServerStrip />
      <MonitoringCard />
      <ServicesCard />
    </>
  );
}
