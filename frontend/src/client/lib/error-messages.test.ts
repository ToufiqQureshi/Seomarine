import { describe, expect, it } from "vitest";
import { getStandardErrorMessage } from "@/client/lib/error-messages";

describe("getStandardErrorMessage", () => {
  it("maps known error codes to standard copy", () => {
    expect(getStandardErrorMessage(new Error("PAYMENT_REQUIRED"))).toBe(
      "An active hosted subscription is required before you can use Uplumo.",
    );
    expect(getStandardErrorMessage(new Error("KEYWORDS_UNAVAILABLE"))).toBe(
      "Keyword research is not configured on this server yet. Contact support for help.",
    );
  });

  it("returns custom messages when the error is not a shared code", () => {
    expect(
      getStandardErrorMessage(new Error("Something failed: try again")),
    ).toBe("Something failed: try again");
  });
});
