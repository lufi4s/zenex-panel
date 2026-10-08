import { BrandingCard } from "@/components/BrandingCard";
import { PageHeader } from "@/components/PageHeader";

/** Panel settings. Administrators only. */
export function SettingsPage() {
  return (
    <>
      <PageHeader
        title="Settings"
        description="Change how the panel is named and coloured for your team."
      />
      <BrandingCard />
    </>
  );
}
