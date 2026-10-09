import { AlertSettingsCard } from "@/components/AlertSettingsCard";
import { BackupSettingsCard } from "@/components/BackupSettingsCard";
import { BrandingCard } from "@/components/BrandingCard";
import { PageHeader } from "@/components/PageHeader";
import { SiteDefaultsCard } from "@/components/SiteDefaultsCard";
import { UpdateCard } from "@/components/UpdateCard";

/** Panel settings. Administrators only. */
export function SettingsPage() {
  return (
    <>
      <PageHeader
        title="Settings"
        description="Change how the panel is named and coloured for your team, and keep it up to date."
      />
      <UpdateCard />
      <SiteDefaultsCard />
      <BackupSettingsCard />
      <AlertSettingsCard />
      <BrandingCard />
    </>
  );
}
