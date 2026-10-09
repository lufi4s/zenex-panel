import type { SiteActivity } from "@/api/types";
import { stepLabel } from "@/lib/format";

export type ActivityTone = "info" | "success" | "danger" | "neutral";

export interface ActivityText {
  /** The short line shown on the website list, for example "Backing up · 45%". */
  text: string;
  tone: ActivityTone;
  /** True while the job is queued or running. */
  running: boolean;
}

// [while it runs, the name of the finished task]
const WORDS: Record<string, [string, string]> = {
  "site.backup": ["Backing up", "Backup"],
  "site.restore": ["Restoring", "Restore"],
  "site.migrate": ["Migrating", "Migration"],
  "site.provision": ["Building", "Build"],
  "site.delete": ["Deleting", "Deletion"],
};

/** What to say about a website's latest job: running with a percent, queued, complete or failed. */
export function describeActivity(a: SiteActivity): ActivityText {
  const [verb, noun] = WORDS[a.type] ?? ["Working", "Task"];
  switch (a.status) {
    case "queued":
      return { text: `${noun} queued`, tone: "neutral", running: true };
    case "running":
      return { text: `${verb} · ${Math.round(a.percent)}%`, tone: "info", running: true };
    case "succeeded":
      return { text: `${noun} complete`, tone: "success", running: false };
    default:
      return { text: `${noun} failed`, tone: "danger", running: false };
  }
}

const STEP_TEXT: Record<string, string> = {
  archive: "Archiving files and database",
  upload: "Uploading to the remote server",
  record: "Saving the backup record",
  save_current: "Saving a copy of the current website",
  download: "Downloading",
  restore: "Restoring files and database",
  create_site: "Building the new website",
  verify: "Checking WordPress",
};

/** A plain-language name for the step that is running. */
export function activityStepText(step: string): string {
  return STEP_TEXT[step] ?? stepLabel(step);
}
