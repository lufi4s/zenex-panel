// A small client-side router built on the browser history API.
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useSyncExternalStore,
  type AnchorHTMLAttributes,
  type ReactNode,
} from "react";

export type Params = Record<string, string>;
export interface NavigateOptions {
  replace?: boolean;
  state?: unknown;
}

const listeners = new Set<() => void>();

function notify() {
  listeners.forEach((listener) => listener());
}

function subscribe(listener: () => void) {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

function currentPath() {
  return window.location.pathname;
}

window.addEventListener("popstate", notify);

/** Moves to another page inside the panel without reloading. */
export function navigate(to: string, options: NavigateOptions = {}) {
  if (options.replace) window.history.replaceState(options.state ?? null, "", to);
  else window.history.pushState(options.state ?? null, "", to);
  notify();
}

export function useLocation() {
  const pathname = useSyncExternalStore(subscribe, currentPath, currentPath);
  return { pathname };
}

export function useNavigate() {
  return useCallback((to: string, options?: NavigateOptions) => navigate(to, options), []);
}

const ParamsContext = createContext<Params>({});

export function useParams(): Params {
  return useContext(ParamsContext);
}

type AnchorProps = AnchorHTMLAttributes<HTMLAnchorElement>;

/** A link that stays inside the panel for normal clicks. */
export function Link({ to, onClick, ...rest }: AnchorProps & { to: string }) {
  return (
    <a
      href={to}
      {...rest}
      onClick={(event) => {
        onClick?.(event);
        const plain =
          event.button === 0 &&
          !event.metaKey &&
          !event.ctrlKey &&
          !event.shiftKey &&
          !event.altKey;
        if (event.defaultPrevented || !plain) return;
        event.preventDefault();
        navigate(to);
      }}
    />
  );
}

/** A link that marks itself active when its page is open. */
export function NavLink({
  to,
  end = false,
  className,
  children,
  ...rest
}: AnchorProps & { to: string; end?: boolean }) {
  const { pathname } = useLocation();
  const active = end ? pathname === to : pathname === to || pathname.startsWith(`${to}/`);
  return (
    <Link
      to={to}
      aria-current={active ? "page" : undefined}
      data-active={active || undefined}
      className={className}
      {...rest}
    >
      {children}
    </Link>
  );
}

/** Sends the visitor somewhere else as soon as it renders. */
export function Navigate({
  to,
  replace = false,
}: {
  to: string;
  replace?: boolean;
  state?: unknown;
}) {
  useEffect(() => {
    navigate(to, { replace });
  }, [to, replace]);
  return null;
}

/** Matches "/websites/:id" against a path. Returns the values of the named parts, or null. */
export function matchPath(pattern: string, pathname: string): Params | null {
  const want = pattern.split("/").filter(Boolean);
  const got = pathname.split("/").filter(Boolean);
  if (want.length !== got.length) return null;
  const params: Params = {};
  for (let i = 0; i < want.length; i++) {
    if (want[i].startsWith(":")) {
      params[want[i].slice(1)] = decodeURIComponent(got[i]);
    } else if (want[i] !== got[i]) {
      return null;
    }
  }
  return params;
}

export interface RouteDef {
  path: string;
  element: ReactNode;
}

/** Renders the first route that matches the current address, or the fallback. */
export function Routes({ routes, fallback }: { routes: RouteDef[]; fallback: ReactNode }) {
  const { pathname } = useLocation();
  for (const route of routes) {
    const params = matchPath(route.path, pathname);
    if (params) {
      return <ParamsContext.Provider value={params}>{route.element}</ParamsContext.Provider>;
    }
  }
  return <>{fallback}</>;
}
