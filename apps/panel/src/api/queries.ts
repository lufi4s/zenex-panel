import { QueryClient, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiRequest, newIdempotencyKey, ApiError } from "./client";
import type { Domain, JobDetail, Metrics, Site, SiteCredentials, User } from "./types";

export const queryClient = new QueryClient({
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
    mutationFn: (apex: string) =>
      apiRequest<Domain>("/api/v1/domains", { method: "POST", body: { apex } }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.domains }),
  });
}

export function useCheckDomain() {
  const qc = useQueryClient();
  return useMutation({
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
    mutationFn: (input: { id: string; action: "suspend" | "resume" | "php-restart" }) =>
      apiRequest<Site | void>(`/api/v1/sites/${input.id}/${input.action}`, { method: "POST" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.sites }),
  });
}

export function useChangePHP() {
  const qc = useQueryClient();
  return useMutation({
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
    mutationFn: (id: string) =>
      apiRequest<{ job_id: string }>(`/api/v1/sites/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: keys.sites }),
  });
}

export function useCredentials() {
  return useMutation({
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
    mutationFn: (id: string) =>
      apiRequest<{ job_id: string }>(`/api/v1/jobs/${id}/retry`, { method: "POST" }),
    onSuccess: (_result, id) => {
      qc.invalidateQueries({ queryKey: keys.job(id) });
      qc.invalidateQueries({ queryKey: keys.sites });
    },
  });
}
