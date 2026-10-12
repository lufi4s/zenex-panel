/** An error the API returned on purpose. Its message is safe to show to the user. */
export class ApiError extends Error {
  readonly status: number;
  readonly code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

/** Header the API requires on every state-changing request (CSRF protection). */
const CSRF_HEADER = { "X-Requested-With": "zenex" } as const;

/** Calls the panel API. Errors carry the API's own message, never raw server output. */
export async function apiRequest<T>(
  path: string,
  options: { method?: string; body?: unknown } = {},
): Promise<T> {
  const headers: Record<string, string> = { ...CSRF_HEADER };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";

  const res = await fetch(path, {
    method: options.method ?? "GET",
    headers,
    credentials: "same-origin",
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });
  if (res.status === 204) return undefined as T;

  const body = (await res.json().catch(() => null)) as {
    error?: { code?: string; message?: string };
  } | null;
  if (!res.ok) {
    throw new ApiError(
      res.status,
      body?.error?.code ?? "request_failed",
      body?.error?.message ?? "Something went wrong. Please try again.",
    );
  }
  return body as T;
}
