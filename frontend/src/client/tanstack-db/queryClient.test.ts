import { describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { ApiError } from "@/client/lib/uplumoApi";
import { queryClient } from "@/client/tanstack-db/queryClient";

vi.mock("sonner", () => ({ toast: { error: vi.fn() } }));

function runFailingMutation(
  options: { onError?: () => void; meta?: { errorToast?: false } } = {},
) {
  return queryClient
    .getMutationCache()
    .build(queryClient, {
      mutationFn: () => Promise.reject(new Error("RATE_LIMITED")),
      ...options,
    })
    .execute(undefined)
    .catch(() => undefined);
}

describe("default mutation error toast", () => {
  it("shows the standard message for a mutation without its own handling", async () => {
    await runFailingMutation();
    expect(toast.error).toHaveBeenCalledWith(
      "Too many requests. Please wait and try again.",
    );
  });

  it("stays quiet when the mutation has its own onError or opts out", async () => {
    await runFailingMutation({ onError: () => undefined });
    await runFailingMutation({ meta: { errorToast: false } });
    expect(toast.error).not.toHaveBeenCalled();
  });
});

describe("queryClient", () => {
  it("does not retry 4xx API failures", () => {
    const retry = queryClient.getDefaultOptions().queries?.retry;
    expect(typeof retry).toBe("function");
    if (typeof retry !== "function")
      throw new Error("retry must be a function");
    expect(retry(0, new ApiError(404, "not_found", "missing"))).toBe(false);
  });
});
