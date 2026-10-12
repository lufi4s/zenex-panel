import { QueryClient, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { apiRequest } from "@/lib/api";
import type { Metrics, User } from "@/lib/types";

/** One query client for the whole panel. Idle data refreshes slowly and pauses while the tab is hidden. */
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: { staleTime: 10_000, refetchOnWindowFocus: true, retry: 1 },
  },
});

const keys = {
  me: ["auth", "me"] as const,
  metrics: ["system", "metrics"] as const,
};

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

export function useMetrics() {
  return useQuery({
    queryKey: keys.metrics,
    queryFn: () => apiRequest<Metrics>("/api/v1/system/metrics"),
    refetchInterval: () => (document.hidden ? false : 15_000),
  });
}
