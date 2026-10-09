import type { ApiErrorBody } from "./types";

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

/**
 * Reads the API's structured error. Falls back to a generic message when the
 * body is missing or not JSON, so the UI never shows raw server output.
 */
export function errorMessageFrom(body: unknown, fallback: string): string {
  const candidate = body as Partial<ApiErrorBody> | null;
  const message = candidate?.error?.message;
  return typeof message === "string" && message.length > 0 ? message : fallback;
}

export async function apiRequest<T>(
  path: string,
  options: { method?: string; body?: unknown; idempotencyKey?: string } = {},
): Promise<T> {
  const method = options.method ?? "GET";
  const headers: Record<string, string> = { ...CSRF_HEADER };
  if (options.body !== undefined) headers["Content-Type"] = "application/json";
  if (options.idempotencyKey) headers["Idempotency-Key"] = options.idempotencyKey;

  const res = await fetch(path, {
    method,
    headers,
    credentials: "same-origin",
    body: options.body === undefined ? undefined : JSON.stringify(options.body),
  });

  if (res.status === 204) return undefined as T;

  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    body = null;
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      (body as Partial<ApiErrorBody> | null)?.error?.code ?? "request_failed",
      errorMessageFrom(body, "Something went wrong. Please try again."),
    );
  }
  return body as T;
}

/** Sends form data, for file uploads. Errors are handled the same way as apiRequest. */
export async function apiUpload<T>(path: string, form: FormData): Promise<T> {
  const res = await fetch(path, {
    method: "POST",
    headers: { ...CSRF_HEADER },
    credentials: "same-origin",
    body: form,
  });
  let body: unknown = null;
  try {
    body = await res.json();
  } catch {
    body = null;
  }
  if (!res.ok) {
    throw new ApiError(
      res.status,
      (body as Partial<ApiErrorBody> | null)?.error?.code ?? "request_failed",
      errorMessageFrom(body, "The file could not be uploaded."),
    );
  }
  return body as T;
}

/** A unique key for one create request, so a double click never makes two sites. */
export function newIdempotencyKey(): string {
  return crypto.randomUUID();
}

/**
 * A short, plain-language message for any failure. Technical details from the
 * server are never shown here, only the API's own sentence.
 */
export function describeError(err: unknown): string {
  if (err instanceof ApiError) return err.message;
  if (err instanceof TypeError) {
    return "Cannot reach the panel. Check your internet connection and try again.";
  }
  return "Something unexpected happened. Please try again.";
}
