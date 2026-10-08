// A small query cache: keyed reads with polling, mutations with callbacks, and prefix invalidation.
// It covers what the panel needs without a data-fetching library.
import { useCallback, useEffect, useReducer, useRef, useState } from "react";
import { ApiError, describeError } from "./client";
import { toast } from "../lib/toast";

type Key = readonly unknown[];
type Kind = "data" | "refetch";
type Listener = (kind: Kind) => void;

interface Entry {
  key: Key;
  data: unknown;
  error: unknown;
  status: "pending" | "success" | "error";
  fetchedAt: number;
  inflight: Promise<void> | null;
  listeners: Set<Listener>;
}

const cache = new Map<string, Entry>();
const DEFAULT_STALE_MS = 2_000;

const hashOf = (key: Key) => JSON.stringify(key);

function entryFor(key: Key): Entry {
  const hash = hashOf(key);
  let entry = cache.get(hash);
  if (!entry) {
    entry = {
      key,
      data: undefined,
      error: null,
      status: "pending",
      fetchedAt: 0,
      inflight: null,
      listeners: new Set(),
    };
    cache.set(hash, entry);
  }
  return entry;
}

function emit(entry: Entry, kind: Kind) {
  entry.listeners.forEach((listener) => listener(kind));
}

function startsWith(key: Key, prefix: Key): boolean {
  return (
    prefix.length <= key.length && prefix.every((part, i) => hashOf([part]) === hashOf([key[i]]))
  );
}

/** Retry rule: never retry a refused request or an expired session; retry server faults once. */
function defaultRetry(failures: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status < 500) return false;
  return failures <= 1;
}

function runQuery(
  entry: Entry,
  queryFn: () => Promise<unknown>,
  retry: boolean | ((failures: number, error: unknown) => boolean) | undefined,
): Promise<void> {
  if (entry.inflight) return entry.inflight;
  entry.inflight = (async () => {
    let failures = 0;
    for (;;) {
      try {
        entry.data = await queryFn();
        entry.error = null;
        entry.status = "success";
        break;
      } catch (error) {
        failures += 1;
        const again =
          typeof retry === "function"
            ? retry(failures, error)
            : retry === undefined
              ? defaultRetry(failures, error)
              : retry;
        if (!again) {
          entry.error = error;
          entry.status = "error";
          break;
        }
        await new Promise((resolve) => setTimeout(resolve, 800));
      }
    }
    entry.fetchedAt = Date.now();
    entry.inflight = null;
    emit(entry, "data");
  })();
  return entry.inflight;
}

// ---------------------------------------------------------------------------
// Client: the shared cache operations (used outside hooks as well)
// ---------------------------------------------------------------------------

export const queryClient = {
  /** Marks matching queries stale and refetches the ones on screen. */
  invalidateQueries({ queryKey }: { queryKey: Key }): Promise<void> {
    cache.forEach((entry) => {
      if (!startsWith(entry.key, queryKey)) return;
      entry.fetchedAt = 0;
      emit(entry, "refetch");
    });
    return Promise.resolve();
  },
  setQueryData(key: Key, data: unknown) {
    const entry = entryFor(key);
    entry.data = data;
    entry.error = null;
    entry.status = "success";
    entry.fetchedAt = Date.now();
    emit(entry, "data");
  },
  /** Forgets every cached answer, then refetches what is on screen (used at sign-out). */
  clear() {
    cache.forEach((entry) => {
      entry.data = undefined;
      entry.error = null;
      entry.status = "pending";
      entry.fetchedAt = 0;
      emit(entry, "refetch");
    });
  },
};

export function useQueryClient() {
  return queryClient;
}

/** Test helper: drops every cached answer without notifying screens. */
export function resetQueryCache() {
  cache.clear();
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

export interface QueryOptions<T> {
  queryKey: Key;
  queryFn: () => Promise<T>;
  enabled?: boolean;
  staleTime?: number;
  retry?: boolean | ((failures: number, error: unknown) => boolean);
  refetchInterval?:
    | number
    | false
    | ((query: { state: { data: T | undefined } }) => number | false);
}

export function useQuery<T>(opts: QueryOptions<T>) {
  const entry = entryFor(opts.queryKey);
  const [, rerender] = useReducer((n: number) => n + 1, 0);
  const latest = useRef(opts);
  latest.current = opts;

  const enabled = opts.enabled ?? true;
  const staleTime = opts.staleTime ?? DEFAULT_STALE_MS;
  const hash = hashOf(opts.queryKey);

  const refetch = useCallback(
    () => runQuery(entry, latest.current.queryFn as () => Promise<unknown>, latest.current.retry),
    [entry],
  );

  // Re-render on every change to this entry; refetch when invalidated.
  useEffect(() => {
    const listener: Listener = (kind) => {
      if (kind === "refetch" && (latest.current.enabled ?? true)) void refetch();
      rerender();
    };
    entry.listeners.add(listener);
    return () => {
      entry.listeners.delete(listener);
    };
  }, [entry, refetch]);

  // Fetch when the key changes or the cached answer is old.
  useEffect(() => {
    if (!enabled) return;
    const fresh = entry.fetchedAt > 0 && Date.now() - entry.fetchedAt < staleTime;
    if (!fresh) void refetch();
    // The key hash identifies the entry; the other values are read through `entry`.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hash, enabled]);

  const interval =
    typeof opts.refetchInterval === "function"
      ? opts.refetchInterval({ state: { data: entry.data as T | undefined } })
      : opts.refetchInterval;

  useEffect(() => {
    if (!enabled || !interval) return;
    const id = window.setInterval(() => void refetch(), interval);
    return () => window.clearInterval(id);
  }, [enabled, interval, refetch]);

  // Refresh when the browser tab comes back into view, if the answer is old.
  useEffect(() => {
    if (!enabled) return;
    const onFocus = () => {
      if (Date.now() - entry.fetchedAt >= staleTime) void refetch();
    };
    window.addEventListener("focus", onFocus);
    return () => window.removeEventListener("focus", onFocus);
  }, [enabled, entry, refetch, staleTime]);

  const data = entry.data as T | undefined;
  return {
    data,
    error: entry.error,
    isPending: data === undefined && entry.status === "pending",
    isError: entry.status === "error",
    isSuccess: entry.status === "success",
    refetch: () => {
      void refetch();
    },
  };
}

// ---------------------------------------------------------------------------
// Infinite lists (pages loaded on demand, first page refreshed by polling)
// ---------------------------------------------------------------------------

export interface InfiniteOptions<T, P> {
  queryKey: Key;
  initialPageParam: P;
  queryFn: (ctx: { pageParam: P }) => Promise<T>;
  getNextPageParam: (last: T) => P | undefined;
  refetchInterval?: number | false;
}

interface InfiniteState<T, P> {
  pages: T[];
  params: P[];
  error: unknown;
  status: "pending" | "success" | "error";
  fetchingNext: boolean;
}

export function useInfiniteQuery<T, P>(opts: InfiniteOptions<T, P>) {
  const latest = useRef(opts);
  latest.current = opts;
  const hash = hashOf(opts.queryKey);
  const initial: InfiniteState<T, P> = {
    pages: [],
    params: [],
    error: null,
    status: "pending",
    fetchingNext: false,
  };
  const [state, setState] = useState<InfiniteState<T, P>>(initial);
  const stateRef = useRef(state);
  stateRef.current = state;
  const mounted = useRef(true);
  const requestSeq = useRef(0);

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const loadFirst = useCallback(async () => {
    const seq = ++requestSeq.current;
    const { queryFn, initialPageParam } = latest.current;
    try {
      const page = await queryFn({ pageParam: initialPageParam });
      if (!mounted.current || seq !== requestSeq.current) return;
      setState((s) =>
        s.pages.length > 0
          ? { ...s, pages: [page, ...s.pages.slice(1)], status: "success", error: null }
          : { ...s, pages: [page], params: [initialPageParam], status: "success", error: null },
      );
    } catch (error) {
      if (!mounted.current || seq !== requestSeq.current) return;
      setState((s) => ({ ...s, error, status: "error" }));
    }
  }, []);

  const fetchNextPage = useCallback(async () => {
    const current = stateRef.current;
    const last = current.pages[current.pages.length - 1];
    if (last === undefined || current.fetchingNext) return;
    const next = latest.current.getNextPageParam(last);
    if (next === undefined) return;
    setState((s) => ({ ...s, fetchingNext: true }));
    try {
      const page = await latest.current.queryFn({ pageParam: next });
      if (!mounted.current) return;
      setState((s) => ({
        ...s,
        pages: [...s.pages, page],
        params: [...s.params, next],
        fetchingNext: false,
      }));
    } catch (error) {
      if (!mounted.current) return;
      setState((s) => ({ ...s, error, fetchingNext: false }));
    }
  }, []);

  // A new key starts from an empty list.
  useEffect(() => {
    setState(initial);
    void loadFirst();
    // `initial` is rebuilt each render; the key hash is the real dependency.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [hash, loadFirst]);

  useEffect(() => {
    if (!opts.refetchInterval) return;
    const id = window.setInterval(() => void loadFirst(), opts.refetchInterval);
    return () => window.clearInterval(id);
  }, [opts.refetchInterval, loadFirst]);

  const last = state.pages[state.pages.length - 1];
  const hasNextPage = last !== undefined && opts.getNextPageParam(last) !== undefined;

  return {
    data: state.pages.length > 0 ? { pages: state.pages } : undefined,
    error: state.error,
    isPending: state.status === "pending" && state.pages.length === 0,
    isError: state.status === "error",
    hasNextPage,
    isFetchingNextPage: state.fetchingNext,
    fetchNextPage,
  };
}

// ---------------------------------------------------------------------------
// Mutations
// ---------------------------------------------------------------------------

export interface MutationCallbacks<TData, TVars> {
  onSuccess?: (data: TData, vars: TVars) => void;
  onError?: (error: unknown, vars: TVars) => void;
  onSettled?: (data: TData | undefined, error: unknown, vars: TVars) => void;
}

export interface MutationOptions<TVars, TData> {
  mutationFn: (vars: TVars) => Promise<TData>;
  onSuccess?: (data: TData, vars: TVars) => void | Promise<unknown>;
  onError?: (error: unknown, vars: TVars) => void | Promise<unknown>;
  onSettled?: (data: TData | undefined, error: unknown, vars: TVars) => void | Promise<unknown>;
  meta?: { silent?: boolean };
}

export function useMutation<TVars = void, TData = unknown>(opts: MutationOptions<TVars, TData>) {
  const latest = useRef(opts);
  latest.current = opts;
  const mounted = useRef(true);
  const [state, setState] = useState<{
    status: "idle" | "pending" | "success" | "error";
    data?: TData;
    error?: unknown;
  }>({ status: "idle" });

  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
    };
  }, []);

  const run = useCallback(async (vars: TVars): Promise<TData> => {
    const o = latest.current;
    setState({ status: "pending" });
    try {
      const data = await o.mutationFn(vars);
      await o.onSuccess?.(data, vars);
      if (mounted.current) setState({ status: "success", data });
      await o.onSettled?.(data, null, vars);
      return data;
    } catch (error) {
      if (mounted.current) setState({ status: "error", error });
      if (!o.meta?.silent) toast.error(describeError(error));
      await o.onError?.(error, vars);
      await o.onSettled?.(undefined, error, vars);
      throw error;
    }
  }, []);

  const mutateAsync = run;

  const mutate = useCallback(
    (vars: TVars, callbacks?: MutationCallbacks<TData, TVars>) => {
      // Errors are shown by the toast and the callbacks, so the promise is not left unhandled.
      run(vars).then(
        (data) => {
          callbacks?.onSuccess?.(data, vars);
          callbacks?.onSettled?.(data, null, vars);
        },
        (error: unknown) => {
          callbacks?.onError?.(error, vars);
          callbacks?.onSettled?.(undefined, error, vars);
        },
      );
    },
    [run],
  );

  return {
    mutate,
    mutateAsync,
    isPending: state.status === "pending",
    isError: state.status === "error",
    isSuccess: state.status === "success",
    error: state.error,
    data: state.data,
    reset: () => setState({ status: "idle" }),
  };
}
