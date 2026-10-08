import {
  MutationCache,
  QueryClient,
  useInfiniteQuery,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { apiRequest, describeError, newIdempotencyKey, ApiError } from "./client";
import { toast } from "../lib/toast";
import type {
  ActivityPage,
  Branding,
  ActivityResult,
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

export const queryClient = new QueryClient({
  mutationCache: new MutationCache({
    onError: (error, _variables, _context, mutation) => {
      // Forms that already show their own error set meta.silent to avoid a second message.
      if (mutation.meta?.silent) return;
      toast.error(describeError(error));
    },
  }),
  defaultOptions: {
    queries: {
      retry: (failureCount, error) => {
        // Never retry an expired session or a refused request.
        if (error instanceof ApiError && error.status < 500) return false;
        return failureCount < 1;
      },
      refetchOnWindowFocus: true,
      staleTime: 2_000,
    },
  },
});

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
    mutationFn: (input: Branding) =>
      apiRequest<Branding>("/api/v1/branding", { method: "PUT", body: input }),
    meta: { silent: true },
    onSuccess: (saved) => {
      qc.setQueryData(["branding"], saved);
      toast.success("Branding saved");
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
