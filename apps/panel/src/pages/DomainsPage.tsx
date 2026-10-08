import { DomainsCard } from "@/components/DomainsCard";
import { PageHeader } from "@/components/PageHeader";

/** Domains that point at this server, with DNS checks and removal. */
export function DomainsPage() {
  return (
    <>
      <PageHeader
        title="Domains"
        description="Connect a domain, point its DNS at this server, then use it for a website."
      />
      <DomainsCard />
    </>
  );
}
