import {
  beforeEach,
  describe,
  expect,
  it,
  vi,
  type MockInstance,
} from "vitest";
import { z } from "zod";
import { ApiError, apiRequest, shouldRetryApiError } from "./seomarineApi";

const schema = z.object({ plan: z.enum(["free", "pro"]) });
// restoreMocks undoes the spy after each test, so each test spies again.
let fetchMock: MockInstance<typeof fetch>;

function respond(status: number, body: string) {
  fetchMock.mockResolvedValue(
    new Response(body, {
      status,
      headers: { "Content-Type": "application/json" },
    }),
  );
}

function errorBody(code: string, message: string) {
  return JSON.stringify({ error: { code, message } });
}

async function rejection(promise: Promise<unknown>): Promise<ApiError> {
  const error = await promise.catch((reason: unknown) => reason);
  if (!(error instanceof ApiError)) {
    throw new Error(`expected an ApiError, got ${String(error)}`);
  }
  return error;
}

describe("apiRequest", () => {
  beforeEach(() => {
    fetchMock = vi.spyOn(globalThis, "fetch");
    respond(200, JSON.stringify({ plan: "pro" }));
  });

  it("returns the validated body and sends the session cookie", async () => {
    await expect(apiRequest("/api/v1/billing/status", schema)).resolves.toEqual(
      { plan: "pro" },
    );
    expect(fetchMock.mock.calls[0][1]).toMatchObject({
      credentials: "same-origin",
    });
  });

  it("rejects a 200 whose body breaks the contract", async () => {
    respond(200, JSON.stringify({ plan: "enterprise" }));
    const error = await rejection(apiRequest("/x", schema));
    expect(error.code).toBe("invalid_response");
  });

  it.each([
    {
      name: "a 404 shows the server's message",
      status: 404,
      body: errorBody("not_found", "Project not found."),
      code: "not_found",
      message: "Project not found.",
    },
    {
      name: "a 401 asks the user to sign in, whatever the server says",
      status: 401,
      body: errorBody("unauthenticated", "missing session cookie"),
      code: "unauthenticated",
      message: "Your session has ended. Sign in again.",
    },
    {
      name: "a 429 asks the user to wait",
      status: 429,
      body: errorBody("rate_limited", "slow down"),
      code: "rate_limited",
      message: "Too many requests. Wait a minute and try again.",
    },
    {
      name: "a 500 hides server internals",
      status: 500,
      body: errorBody("internal", "pq: relation does not exist"),
      code: "internal",
      message: "Something went wrong on our side. Try again.",
    },
    {
      name: "a 502 HTML page from a proxy still becomes an ApiError",
      status: 502,
      body: "<html>Bad gateway</html>",
      code: "unknown",
      message: "Something went wrong on our side. Try again.",
    },
    {
      name: "a 400 with an empty message falls back to generic text",
      status: 400,
      body: errorBody("bad_request", ""),
      code: "bad_request",
      message: "The request failed. Try again.",
    },
  ])("$name", async ({ status, body, code, message }) => {
    respond(status, body);
    const error = await rejection(apiRequest("/x", schema));
    expect(error).toMatchObject({ status, code, message });
  });
});

describe("shouldRetryApiError", () => {
  it.each([
    { error: new TypeError("Failed to fetch"), failures: 0, retry: true },
    { error: new TypeError("Failed to fetch"), failures: 2, retry: false },
    { error: new ApiError(503, "unavailable", "x"), failures: 1, retry: true },
    {
      error: new ApiError(401, "unauthenticated", "x"),
      failures: 0,
      retry: false,
    },
    {
      error: new ApiError(429, "rate_limited", "x"),
      failures: 0,
      retry: false,
    },
    {
      error: new ApiError(0, "invalid_response", "x"),
      failures: 0,
      retry: false,
    },
  ])(
    "$error.message after $failures failures -> $retry",
    ({ error, failures, retry }) => {
      expect(shouldRetryApiError(failures, error)).toBe(retry);
    },
  );
});
