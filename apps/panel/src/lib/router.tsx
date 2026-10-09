// A small client-side router on the browser history API, built on Solid signals.
import {
  createContext,
  createMemo,
  createSignal,
  onMount,
  Show,
  useContext,
  type JSX,
} from "solid-js";

export type Params = Record<string, string>;
export interface NavigateOptions {
  replace?: boolean;
  state?: unknown;
}

const [pathSignal, setPathSignal] = createSignal(window.location.pathname);

window.addEventListener("popstate", () => setPathSignal(window.location.pathname));

/** Moves to another page inside the panel without reloading. */
export function navigate(to: string, options: NavigateOptions = {}) {
  if (options.replace) window.history.replaceState(options.state ?? null, "", to);
  else window.history.pushState(options.state ?? null, "", to);
  setPathSignal(window.location.pathname);
}

export function useLocation() {
  return {
    get pathname() {
      return pathSignal();
    },
  };
}

export function useNavigate() {
  return (to: string, options?: NavigateOptions) => navigate(to, options);
}

const ParamsContext = createContext<Params>({});

export function useParams(): Params {
  return useContext(ParamsContext);
}

type AnchorProps = JSX.AnchorHTMLAttributes<HTMLAnchorElement>;

/** A link that stays inside the panel for normal clicks. */
export function Link(props: AnchorProps & { to: string }) {
  const onClick = (event: MouseEvent) => {
    const anchorClick = props.onClick as ((e: MouseEvent) => void) | undefined;
    anchorClick?.(event);
    const plain =
      event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
    if (event.defaultPrevented || !plain) return;
    event.preventDefault();
    navigate(props.to);
  };
  return <a {...props} href={props.to} onClick={onClick} />;
}

/** A link that marks itself active when its page is open. */
export function NavLink(props: AnchorProps & { to: string; end?: boolean }) {
  const { pathname } = useLocation();
  const active = () =>
    props.end
      ? pathname === props.to
      : pathname === props.to || pathname.startsWith(`${props.to}/`);
  return (
    <Link
      to={props.to}
      class={props.class}
      onClick={props.onClick}
      aria-current={active() ? "page" : undefined}
      data-active={active() ? "" : undefined}
    >
      {props.children}
    </Link>
  );
}

/** Sends the visitor somewhere else as soon as the page is shown. */
export function Navigate(props: { to: string; replace?: boolean }) {
  onMount(() => navigate(props.to, { replace: props.replace ?? false }));
  return null;
}

/** Matches "/websites/:id" against a path. Returns the named values, or null. */
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
  /** Builds the page. A function, so each page is created only when it is shown. */
  component: () => JSX.Element;
}

/** Renders the first route that matches the current address, or the fallback. */
export function Routes(props: { routes: RouteDef[]; fallback: () => JSX.Element }) {
  const match = createMemo(() => {
    const pathname = pathSignal();
    for (const route of props.routes) {
      const params = matchPath(route.path, pathname);
      if (params) return { route, params };
    }
    return null;
  });
  return (
    <Show when={match()} fallback={props.fallback()}>
      {(m) => (
        <ParamsContext.Provider value={m().params}>{m().route.component()}</ParamsContext.Provider>
      )}
    </Show>
  );
}
