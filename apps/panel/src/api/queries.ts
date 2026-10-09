import { useInfiniteQuery, useMutation, useQuery, useQueryClient } from "./query";
import { ApiError, apiRequest, newIdempotencyKey } from "./client";
import { toast } from "../lib/toast";
import { touchBrandAssets } from "../lib/brand";
import type {
  ActivityPage,
  AlertSettings,
  AlertSettingsInput,
  BackupSettings,
  Branding,
  BrandingInput,
  BrandImageInput,
  SftpSettings,
  ActivityResult,
  SiteBackup,
  SiteDefaults,
  Domain,
  Job,
  JobStep,
  FileContent,
  FileEntry,
  JobDetail,
  JobLogLine,
  Metrics,
  MetricSeries,
  NotificationsPage,
  ServiceState,
  Site,
  SiteCredentials,
  SiteHealth,
  TimeRange,
  User,
} from "./types";

const keys = {
  me: ["me"] as const,
  metrics: ["metrics"] as const,
  domains: ["domains"] as const,
  sites: ["sites"] as const,
  job: (id: string) => ["job", id] as const,
  phpVersions: ["php-versions"] as const,
};

// ---------------------------------------------------------------------------
// Session
// ---------------------------------------------------------------------------

export function useMe() {
  return useQuery({
    queryKey: keys.me,
    queryFn: () => apiRequest<User>("/api/v1/auth/me"),
    retry: false,
  });
}

export function useLogin() {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: (input: { email: string; password: string }) =>
      apiRequest<User>("/api/v1/auth/login", { method: "POST", body: input }),
    onSuccess: (user) => qc.setQueryData(keys.me, user),
  });
}

export function useLogout() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () => apiRequest<void>("/api/v1/auth/logout", { method: "POST" }),
    onSettled: () => qc.clear(),
  });
}

// ---------------------------------------------------------------------------
// Server
// ---------------------------------------------------------------------------

export function useMetrics() {
  return useQuery({
    queryKey: keys.metrics,
    queryFn: () => apiRequest<Metrics>("/api/v1/system/metrics"),
    refetchInterval: 5_000,
  });
}

// ---------------------------------------------------------------------------
// Domains
// ---------------------------------------------------------------------------

export function useDomains() {
  return useQuery({
    queryKey: keys.domains,
    queryFn: () => apiRequest<Domain[]>("/api/v1/domains"),
  });
}

export function useAddDomain() {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: (apex: string) =>
      apiRequest<Domain>("/api/v1/domains", { method: "POST", body: { apex } }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.domains }),
  });
}

export function useCheckDomain() {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: (id: string) =>
      apiRequest<Domain>(`/api/v1/domains/${id}/verify`, { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.domains }),
  });
}

// ---------------------------------------------------------------------------
// Websites
// ---------------------------------------------------------------------------

export function useSites() {
  return useQuery({
    queryKey: keys.sites,
    queryFn: () => apiRequest<Site[]>("/api/v1/sites"),
    refetchInterval: (query) => {
      const sites = query.state.data ?? [];
      return sites.some((s) => s.state === "provisioning" || s.state === "deleting")
        ? 3_000
        : false;
    },
  });
}

export interface CreateSiteResult {
  site: Site;
  job_id: string;
}

export function useCreateSite() {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: (input: { label: string; apex: string }) =>
      apiRequest<CreateSiteResult>("/api/v1/sites", {
        method: "POST",
        body: input,
        // One key per attempt: a double click or a retried request creates one site.
        idempotencyKey: newIdempotencyKey(),
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.sites }),
  });
}

export function useSiteAction() {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: (input: { id: string; action: "suspend" | "resume" | "php-restart" }) =>
      apiRequest<Site | void>(`/api/v1/sites/${input.id}/${input.action}`, { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.sites }),
  });
}

export function useChangePHP() {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: (input: { id: string; version: string }) =>
      apiRequest<Site>(`/api/v1/sites/${input.id}/php`, {
        method: "POST",
        body: { version: input.version },
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.sites }),
  });
}

export function useDeleteSite() {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: (id: string) =>
      apiRequest<{ job_id: string }>(`/api/v1/sites/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.sites }),
  });
}

export function useCredentials() {
  return useMutation({
    meta: { silent: true },
    mutationFn: (id: string) => apiRequest<SiteCredentials>(`/api/v1/sites/${id}/credentials`),
  });
}

export function useSiteLog() {
  return useMutation({
    mutationFn: (id: string) =>
      apiRequest<{ log: string }>(`/api/v1/sites/${id}/logs`).then((r) => r.log),
  });
}

export function usePHPVersions() {
  return useQuery({
    queryKey: keys.phpVersions,
    queryFn: () =>
      apiRequest<{ versions: string[] }>("/api/v1/php-versions").then((r) => r.versions),
    staleTime: Infinity,
  });
}

// ---------------------------------------------------------------------------
// Jobs (build and delete progress)
// ---------------------------------------------------------------------------

export function useJob(id: string | null) {
  return useQuery({
    queryKey: id ? keys.job(id) : ["job", "none"],
    queryFn: () => apiRequest<JobDetail>(`/api/v1/jobs/${id}`),
    enabled: id !== null,
    refetchInterval: (query) => {
      const status = query.state.data?.job.status;
      return status === "queued" || status === "running" ? 2_000 : false;
    },
  });
}

export function useRetryJob() {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: (id: string) =>
      apiRequest<{ job_id: string }>(`/api/v1/jobs/${id}/retry`, { method: "POST" }),
    onSuccess: (_result, id) => {
      qc.invalidateQueries({ queryKey: keys.job(id) });
      qc.invalidateQueries({ queryKey: keys.sites });
    },
  });
}

// ---------------------------------------------------------------------------
// Monitoring, services, activity and job logs
// ---------------------------------------------------------------------------

export function useMetricSeries(range: TimeRange) {
  return useQuery({
    queryKey: ["monitoring", "metrics", range],
    queryFn: () => apiRequest<MetricSeries>(`/api/v1/monitoring/metrics?range=${range}`),
    refetchInterval: 30_000,
  });
}

export function useSiteHealth(range: TimeRange) {
  return useQuery({
    queryKey: ["monitoring", "sites", range],
    queryFn: () => apiRequest<SiteHealth[]>(`/api/v1/monitoring/sites?range=${range}`),
    refetchInterval: 60_000,
  });
}

export function useServices() {
  return useQuery({
    queryKey: ["monitoring", "services"],
    queryFn: () => apiRequest<ServiceState[]>("/api/v1/monitoring/services"),
    refetchInterval: 30_000,
  });
}

export interface ActivityFilter {
  action: string;
  result: ActivityResult | "";
}

export function useActivity(filter: ActivityFilter, live: boolean) {
  return useInfiniteQuery({
    queryKey: ["activity", filter.action, filter.result],
    initialPageParam: 0,
    queryFn: ({ pageParam }) => {
      const params = new URLSearchParams({ limit: "50" });
      if (pageParam) params.set("before", String(pageParam));
      if (filter.action) params.set("action", filter.action);
      if (filter.result) params.set("result", filter.result);
      return apiRequest<ActivityPage>(`/api/v1/activity?${params.toString()}`);
    },
    getNextPageParam: (last) => last.next_before ?? undefined,
    refetchInterval: live ? 15_000 : false,
  });
}

export function useJobLogs(jobId: string | null, enabled: boolean) {
  return useQuery({
    queryKey: ["job-logs", jobId],
    queryFn: () => apiRequest<JobLogLine[]>(`/api/v1/jobs/${jobId}/logs`),
    enabled: enabled && jobId !== null,
    refetchInterval: enabled ? 2_000 : false,
  });
}

// ---------------------------------------------------------------------------
// Notifications and domain removal
// ---------------------------------------------------------------------------

export function useNotifications() {
  return useQuery({
    queryKey: ["notifications"],
    queryFn: () => apiRequest<NotificationsPage>("/api/v1/notifications?limit=30"),
    refetchInterval: 30_000,
  });
}

export function useMarkNotificationsRead() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { ids?: number[]; all?: boolean }) =>
      apiRequest<void>("/api/v1/notifications/read", {
        method: "POST",
        body: { ids: input.ids ?? [], all: input.all ?? false },
      }),
    meta: { silent: true },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["notifications"] }),
  });
}

export function useDeleteDomain() {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: (id: string) => apiRequest<void>(`/api/v1/domains/${id}`, { method: "DELETE" }),
    onSuccess: () => {
      toast.success("Domain removed");
      qc.invalidateQueries({ queryKey: keys.domains });
      qc.invalidateQueries({ queryKey: keys.sites });
    },
  });
}

// ---------------------------------------------------------------------------
// Website file manager
// ---------------------------------------------------------------------------

export function useFolder(siteId: string, path: string, enabled: boolean) {
  return useQuery({
    queryKey: ["files", siteId, path],
    queryFn: () =>
      apiRequest<FileEntry[]>(`/api/v1/sites/${siteId}/files?path=${encodeURIComponent(path)}`),
    enabled,
  });
}

export function useFileContent(siteId: string, path: string | null) {
  return useQuery({
    queryKey: ["file", siteId, path],
    queryFn: () =>
      apiRequest<FileContent>(
        `/api/v1/sites/${siteId}/file?path=${encodeURIComponent(path ?? "")}`,
      ),
    enabled: path !== null,
    staleTime: 0,
    retry: false,
  });
}

export function useSaveFile(siteId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: { path: string; content: string }) =>
      apiRequest<void>(`/api/v1/sites/${siteId}/file`, { method: "PUT", body: input }),
    meta: { silent: true },
    onSuccess: (_result, input) => {
      toast.success(`Saved ${input.path.split("/").pop() ?? input.path}`);
      qc.invalidateQueries({ queryKey: ["files", siteId] });
      qc.invalidateQueries({ queryKey: ["file", siteId, input.path] });
    },
  });
}

export function useCreateFolder(siteId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (path: string) =>
      apiRequest<void>(`/api/v1/sites/${siteId}/folders`, { method: "POST", body: { path } }),
    meta: { silent: true },
    onSuccess: () => qc.invalidateQueries({ queryKey: ["files", siteId] }),
  });
}

export function useDeleteFile(siteId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (path: string) =>
      apiRequest<void>(`/api/v1/sites/${siteId}/files?path=${encodeURIComponent(path)}`, {
        method: "DELETE",
      }),
    meta: { silent: true },
    onSuccess: (_result, path) => {
      toast.success(`Deleted ${path.split("/").pop() ?? path}`);
      qc.invalidateQueries({ queryKey: ["files", siteId] });
    },
  });
}

// ---------------------------------------------------------------------------
// Branding
// ---------------------------------------------------------------------------

export function useBranding() {
  return useQuery({
    queryKey: ["branding"],
    queryFn: () => apiRequest<Branding>("/api/v1/branding"),
    staleTime: 5 * 60_000,
    retry: false,
  });
}

export function useSaveBranding() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: BrandingInput) =>
      apiRequest<Branding | void>("/api/v1/branding", { method: "PUT", body: input }),
    meta: { silent: true },
    onSuccess: () => {
      // Refetch so the has_logo and has_favicon flags come from the server.
      void qc.invalidateQueries({ queryKey: ["branding"] });
      toast.success("Branding saved");
    },
  });
}

export type BrandImageKind = "logo" | "favicon";

/** Uploads the logo or favicon. Cached copies are busted by touching the asset version. */
export function useSetBrandImage(kind: BrandImageKind) {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: (input: BrandImageInput) =>
      apiRequest<void>(`/api/v1/branding/${kind}`, { method: "PUT", body: input }),
    onSuccess: () => {
      touchBrandAssets();
      void qc.invalidateQueries({ queryKey: ["branding"] });
      toast.success(kind === "logo" ? "Logo uploaded" : "Favicon uploaded");
    },
  });
}

export function useRemoveBrandImage(kind: BrandImageKind) {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: () => apiRequest<void>(`/api/v1/branding/${kind}`, { method: "DELETE" }),
    onSuccess: () => {
      touchBrandAssets();
      void qc.invalidateQueries({ queryKey: ["branding"] });
      toast.success(kind === "logo" ? "Logo removed" : "Favicon removed");
    },
  });
}

// ---------------------------------------------------------------------------
// Single website (its own page)
// ---------------------------------------------------------------------------

export interface SiteDetail {
  site: Site;
  job?: Job;
  steps?: JobStep[];
}

export function useSite(id: string) {
  return useQuery({
    queryKey: ["site", id],
    queryFn: () => apiRequest<SiteDetail>(`/api/v1/sites/${id}`),
    refetchInterval: (query) => {
      const job = query.state.data?.job;
      return job && (job.status === "queued" || job.status === "running") ? 2_000 : 15_000;
    },
  });
}

export function useSiteLogs(id: string, enabled: boolean) {
  return useQuery({
    queryKey: ["site-logs", id],
    queryFn: () => apiRequest<{ log: string }>(`/api/v1/sites/${id}/logs`).then((r) => r.log),
    enabled,
    refetchInterval: enabled ? 10_000 : false,
  });
}

// ---------------------------------------------------------------------------
// Panel updates (administrators)
// ---------------------------------------------------------------------------

export interface UpdateStatus {
  current: string;
  latest: string;
  update_available: boolean;
  state: "idle" | "running" | "succeeded" | "failed";
  log: string;
  error?: string;
}

export function useSystemUpdate(enabled: boolean) {
  return useQuery({
    queryKey: ["system-update"],
    queryFn: () => apiRequest<UpdateStatus>("/api/v1/system/update"),
    enabled,
    // Check every minute, and every 3 seconds while an update is running.
    refetchInterval: (query) => (query.state.data?.state === "running" ? 3_000 : 60_000),
  });
}

export function useStartUpdate() {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: () => apiRequest<{ state: string }>("/api/v1/system/update", { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["system-update"] }),
  });
}

// ---------------------------------------------------------------------------
// Settings: site defaults, backup schedule, alerts (administrators)
// ---------------------------------------------------------------------------

export function useSettingsDefaults() {
  return useQuery({
    queryKey: ["settings", "defaults"],
    queryFn: () => apiRequest<SiteDefaults>("/api/v1/settings/defaults"),
    retry: false,
  });
}

export function useSaveDefaults() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: SiteDefaults) =>
      apiRequest<SiteDefaults | void>("/api/v1/settings/defaults", { method: "PUT", body: input }),
    meta: { silent: true },
    onSuccess: (saved, input) => {
      qc.setQueryData(["settings", "defaults"], saved ?? input);
      toast.success("Site defaults saved");
    },
  });
}

export function useSettingsBackups() {
  return useQuery({
    queryKey: ["settings", "backups"],
    queryFn: () => apiRequest<BackupSettings>("/api/v1/settings/backups"),
    retry: false,
  });
}

export function useSaveBackups() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: BackupSettings) =>
      apiRequest<BackupSettings | void>("/api/v1/settings/backups", {
        method: "PUT",
        body: input,
      }),
    meta: { silent: true },
    onSuccess: (saved, input) => {
      qc.setQueryData(["settings", "backups"], saved ?? input);
      toast.success("Backup settings saved");
    },
  });
}

/** Starts a backup of every site now. Answers how many sites were started and how many were already running. */
export function useRunBackupsNow() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiRequest<{ started: number; skipped: number }>("/api/v1/settings/backups/run-now", {
        method: "POST",
      }),
    meta: { silent: true },
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: keys.sites });
      void qc.invalidateQueries({ queryKey: ["backups"] });
    },
  });
}

/** The SFTP public key the backups use. A 404 means no key has been generated yet. */
export function useSftpKey() {
  return useQuery({
    queryKey: ["settings", "sftp-key"],
    queryFn: () => apiRequest<{ public_key: string }>("/api/v1/settings/backups/sftp-key"),
    retry: false,
  });
}

/** Generates the SFTP key if needed and returns it. */
export function useSftpPublicKey() {
  const qc = useQueryClient();
  return useMutation({
    meta: { silent: true },
    mutationFn: () =>
      apiRequest<{ public_key: string }>("/api/v1/settings/backups/sftp-key", { method: "POST" }),
    onSuccess: (saved) => qc.setQueryData(["settings", "sftp-key"], saved),
  });
}

/** Checks that the panel can log in to the SFTP server with the current fields. */
export function useTestSftp() {
  return useMutation({
    meta: { silent: true },
    mutationFn: (input: SftpSettings) =>
      apiRequest<{ ok: boolean }>("/api/v1/settings/backups/sftp-test", {
        method: "POST",
        body: input,
      }).then((result) => {
        if (result?.ok !== true) throw new ApiError(502, "sftp_failed", "Connection failed.");
        return result;
      }),
  });
}

export function useSendTestEmail() {
  return useMutation({
    meta: { silent: true },
    mutationFn: () =>
      apiRequest<{ sent: boolean }>("/api/v1/settings/alerts/test-email", { method: "POST" }),
  });
}

export function useSettingsAlerts() {
  return useQuery({
    queryKey: ["settings", "alerts"],
    queryFn: () => apiRequest<AlertSettings>("/api/v1/settings/alerts"),
    retry: false,
  });
}

export function useSaveAlerts() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (input: AlertSettingsInput) =>
      apiRequest<AlertSettings | void>("/api/v1/settings/alerts", { method: "PUT", body: input }),
    meta: { silent: true },
    onSuccess: (saved) => {
      // The saved copy never includes secrets, so refetch unless the API returned it.
      if (saved) qc.setQueryData(["settings", "alerts"], saved);
      else void qc.invalidateQueries({ queryKey: ["settings", "alerts"] });
      toast.success("Alert settings saved");
    },
  });
}

// ---------------------------------------------------------------------------
// Website settings: auto-updates, maintenance mode, backups
// ---------------------------------------------------------------------------

/** Both toggles change the site record, so the list and the site page refresh. */
function invalidateSite(qc: ReturnType<typeof useQueryClient>, siteId: string) {
  void qc.invalidateQueries({ queryKey: keys.sites });
  void qc.invalidateQueries({ queryKey: ["site", siteId] });
}

export function useSetAutoUpdate(siteId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (enabled: boolean) =>
      apiRequest<void>(`/api/v1/sites/${siteId}/auto-update`, {
        method: "PUT",
        body: { enabled },
      }),
    meta: { silent: true },
    onSuccess: () => invalidateSite(qc, siteId),
  });
}

export function useSetMaintenance(siteId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (enabled: boolean) =>
      apiRequest<void>(`/api/v1/sites/${siteId}/maintenance`, {
        method: "PUT",
        body: { enabled },
      }),
    meta: { silent: true },
    onSuccess: () => invalidateSite(qc, siteId),
  });
}

export function useSiteBackups(siteId: string) {
  return useQuery({
    queryKey: ["backups", siteId],
    queryFn: () => apiRequest<SiteBackup[]>(`/api/v1/sites/${siteId}/backups`),
  });
}

export function useBackupNow(siteId: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: () =>
      apiRequest<{ job_id: string }>(`/api/v1/sites/${siteId}/backup`, { method: "POST" }),
    meta: { silent: true },
    onSuccess: () => {
      toast.success("Backup started");
      void qc.invalidateQueries({ queryKey: ["backups", siteId] });
    },
  });
}
