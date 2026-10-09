// A small query cache built on Solid signals: keyed reads with polling, mutations with
// callbacks, prefix invalidation, and an infinite list. Hooks return objects whose
// fields are getters, so reading `query.data` inside JSX stays reactive.
import { createEffect, createSignal, onCleanup, type Accessor } from "solid-js";
import { ApiError, describeError } from "./client";
import { toast } from "../lib/toast";

type Key = readonly unknown[];
type Kind = "data" | "refetch";
type Listener = (kind: Kind) => void;
type Retry = boolean | ((failures: number, error: unknown) => boolean);

interface Entry {
  key: Key;
  data: unknown;
  error: unknown;
  status: "pending" | "success" | "error";
  fetchedAt: number;
  inflight: Promise<void> | null;
  listeners: Set<Listener>;
  version: Accessor<number>;
  bump: () => void;
}

const cache = new Map<string, Entry>();
const DEFAULT_STALE_MS = 2_000;

const hashOf = (key: Key) => JSON.stringify(key);

function entryFor(key: Key): Entry {
  const hash = hashOf(key);
  let entry = cache.get(hash);
  if (!entry) {
    const [version, setVersion] = createSignal(0);
    entry = {
      key,
      data: undefined,
      error: null,
      status: "pending",
      fetchedAt: 0,
      inflight: null,
      listeners: new Set(),
      version,
      bump: () => setVersion((n) => n + 1),
    };
    cache.set(hash, entry);
  }
  return entry;
}

function emit(entry: Entry, kind: Kind) {
  entry.bump();
  entry.listeners.forEach((listener) => listener(kind));
}

function startsWith(key: Key, prefix: Key): boolean {
  return (
    prefix.length <= key.length && prefix.every((part, i) => hashOf([part]) === hashOf([key[i]]))
  );
}

/** Never retry a refused request or an expired session; retry server faults once. */
function defaultRetry(failures: number, error: unknown): boolean {
  if (error instanceof ApiError && error.status < 500) return false;
  return failures <= 1;
}

function runQuery(
  entry: Entry,
  queryFn: () => Promise<unknown>,
  retry: Retry | undefined,
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
// Shared cache operations (also usable outside components)
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

/** Test helper: drops every cached answer. */
export function resetQueryCache() {
  cache.clear();
}

// ---------------------------------------------------------------------------
// Queries
// ---------------------------------------------------------------------------

export interface QueryOptions<T> {
  queryKey: Key;
  queryFn: () => Promise<T>;
  enabled?: boolean | (() => boolean);
  staleTime?: number;
  retry?: Retry;
  refetchInterval?:
    | number
    | false
    | ((query: { state: { data: T | undefined } }) => number | false);
}

export function useQuery<T>(opts: QueryOptions<T>) {
  const entry = entryFor(opts.queryKey);
  const enabled = () =>
    typeof opts.enabled === "function" ? opts.enabled() : (opts.enabled ?? true);
  const staleTime = opts.staleTime ?? DEFAULT_STALE_MS;
  const refetch = () => runQuery(entry, opts.queryFn as () => Promise<unknown>, opts.retry);

  const listener: Listener = (kind) => {
    if (kind === "refetch" && enabled()) void refetch();
    entry.bump();
  };
  entry.listeners.add(listener);
  onCleanup(() => entry.listeners.delete(listener));

  if (enabled()) {
    const fresh = entry.fetchedAt > 0 && Date.now() - entry.fetchedAt < staleTime;
    if (!fresh) void refetch();
  }

  if (opts.refetchInterval !== undefined && opts.refetchInterval !== false) {
    createEffect(() => {
      entry.version();
      const interval =
        typeof opts.refetchInterval === "function"
          ? opts.refetchInterval({ state: { data: entry.data as T | undefined } })
          : opts.refetchInterval;
      if (!enabled() || !interval) return;
      const id = window.setInterval(() => void refetch(), interval);
      onCleanup(() => window.clearInterval(id));
    });
  }

  const onFocus = () => {
    if (enabled() && Date.now() - entry.fetchedAt >= staleTime) void refetch();
  };
  window.addEventListener("focus", onFocus);
  onCleanup(() => window.removeEventListener("focus", onFocus));

  return {
    get data(): T | undefined {
      entry.version();
      return entry.data as T | undefined;
    },
    get error(): unknown {
      entry.version();
      return entry.error;
    },
    get isPending(): boolean {
      entry.version();
      return entry.data === undefined && entry.status === "pending";
    },
    get isError(): boolean {
      entry.version();
      return entry.status === "error";
    },
    get isSuccess(): boolean {
      entry.version();
      return entry.status === "success";
    },
    refetch: () => {
      void refetch();
    },
  };
}

// ---------------------------------------------------------------------------
// Infinite lists (pages loaded on demand; the first page is refreshed by polling)
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
  const [state, setState] = createSignal<InfiniteState<T, P>>({
    pages: [],
    params: [],
    error: null,
    status: "pending",
    fetchingNext: false,
  });
  let requestSeq = 0;

  const loadFirst = async () => {
    const seq = ++requestSeq;
    try {
      const page = await opts.queryFn({ pageParam: opts.initialPageParam });
      if (seq !== requestSeq) return;
      setState((s) =>
        s.pages.length > 0
          ? { ...s, pages: [page, ...s.pages.slice(1)], status: "success", error: null }
          : {
              ...s,
              pages: [page],
              params: [opts.initialPageParam],
              status: "success",
              error: null,
            },
      );
    } catch (error) {
      if (seq !== requestSeq) return;
      setState((s) => ({ ...s, error, status: "error" }));
    }
  };

  const fetchNextPage = async () => {
    const current = state();
    const last = current.pages[current.pages.length - 1];
    if (last === undefined || current.fetchingNext) return;
    const next = opts.getNextPageParam(last);
    if (next === undefined) return;
    setState((s) => ({ ...s, fetchingNext: true }));
    try {
      const page = await opts.queryFn({ pageParam: next });
      setState((s) => ({
        ...s,
        pages: [...s.pages, page],
        params: [...s.params, next],
        fetchingNext: false,
      }));
    } catch (error) {
      setState((s) => ({ ...s, error, fetchingNext: false }));
    }
  };

  void loadFirst();

  if (opts.refetchInterval) {
    const id = window.setInterval(() => void loadFirst(), opts.refetchInterval);
    onCleanup(() => window.clearInterval(id));
  }

  return {
    get data(): { pages: T[] } | undefined {
      const s = state();
      return s.pages.length > 0 ? { pages: s.pages } : undefined;
    },
    get error(): unknown {
      return state().error;
    },
    get isPending(): boolean {
      const s = state();
      return s.status === "pending" && s.pages.length === 0;
    },
    get isError(): boolean {
      return state().status === "error";
    },
    get hasNextPage(): boolean {
      const s = state();
      const last = s.pages[s.pages.length - 1];
      return last !== undefined && opts.getNextPageParam(last) !== undefined;
    },
    get isFetchingNextPage(): boolean {
      return state().fetchingNext;
    },
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
  const [state, setState] = createSignal<{
    status: "idle" | "pending" | "success" | "error";
    data?: TData;
    error?: unknown;
  }>({ status: "idle" });

  const run = async (vars: TVars): Promise<TData> => {
    setState({ status: "pending" });
    try {
      const data = await opts.mutationFn(vars);
      await opts.onSuccess?.(data, vars);
      setState({ status: "success", data });
      await opts.onSettled?.(data, null, vars);
      return data;
    } catch (error) {
      setState({ status: "error", error });
      if (!opts.meta?.silent) toast.error(describeError(error));
      await opts.onError?.(error, vars);
      await opts.onSettled?.(undefined, error, vars);
      throw error;
    }
  };

  const mutate = (vars: TVars, callbacks?: MutationCallbacks<TData, TVars>) => {
    // Errors are shown by the toast and the callbacks, so the promise is never left unhandled.
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
  };

  return {
    mutate,
    mutateAsync: run,
    get isPending() {
      return state().status === "pending";
    },
    get isError() {
      return state().status === "error";
    },
    get isSuccess() {
      return state().status === "success";
    },
    get error() {
      return state().error;
    },
    get data() {
      return state().data;
    },
    reset: () => setState({ status: "idle" }),
  };
}
