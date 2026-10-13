import { z } from "zod";

/**
 * A failed call to the Uplumo Go API. `status` is the HTTP status (0 when
 * the response was not the JSON the contract promises) and `code` the
 * server's error code. `message` is always safe to show to the user.
 */
export class ApiError extends Error {
  constructor(
    readonly status: number,
    readonly code: string,
    message: string,
  ) {
    super(message);
    this.name = "ApiError";
  }
}

const errorBodySchema = z.object({
  error: z.object({ code: z.string(), message: z.string() }),
});

// Server errors whose message is written for the customer, not a leak of
// internals: paid data paused by the daily safety limit.
const CUSTOMER_FACING_SERVER_CODES = new Set(["usage_paused"]);

// Statuses whose server message is not meant for the user, or that need the
// same wording whatever the endpoint.
function messageForStatus(
  status: number,
  code: string,
  serverMessage: string | undefined,
) {
  if (status === 401) return "Your session has ended. Sign in again.";
  if (status === 429) return "Too many requests. Wait a minute and try again.";
  if (
    status >= 500 &&
    !(CUSTOMER_FACING_SERVER_CODES.has(code) && serverMessage)
  )
    return "Something went wrong on our side. Try again.";
  return serverMessage || "The request failed. Try again.";
}

/**
 * Calls the Go API on the same origin with the session cookie and validates
 * the JSON response against `schema`. Throws `ApiError` for an error status
 * or a response that does not match the contract.
 */
export async function apiRequest<T>(
  path: string,
  schema: z.ZodType<T>,
  method: "GET" | "POST" = "GET",
  jsonBody?: unknown,
): Promise<T> {
  if (method === "GET" && jsonBody !== undefined) {
    throw new TypeError("GET requests cannot include a JSON body");
  }
  const requestBody =
    method === "POST" && jsonBody !== undefined
      ? JSON.stringify(jsonBody)
      : undefined;
  const response = await fetch(path, {
    method,
    credentials: "same-origin",
    headers: {
      Accept: "application/json",
      ...(requestBody === undefined
        ? {}
        : { "Content-Type": "application/json" }),
    },
    ...(requestBody === undefined ? {} : { body: requestBody }),
  });
  // A proxy error page or an empty body is not JSON: treat it as no body.
  const body: unknown = await response.json().catch(() => undefined);

  if (!response.ok) {
    const parsed = errorBodySchema.safeParse(body);
    const code = parsed.success ? parsed.data.error.code : "unknown";
    throw new ApiError(
      response.status,
      code,
      messageForStatus(response.status, code, parsed.data?.error.message),
    );
  }

  const parsed = schema.safeParse(body);
  if (!parsed.success) {
    throw new ApiError(
      0,
      "invalid_response",
      "We got an unexpected answer from the server. Reload the page and try again.",
    );
  }
  return parsed.data;
}

/**
 * Retry network failures and 5xx twice. A 4xx or a response that breaks the
 * contract fails the same way on every retry.
 */
export function shouldRetryApiError(failureCount: number, error: unknown) {
  const retryable = !(error instanceof ApiError) || error.status >= 500;
  return retryable && failureCount < 2;
}
