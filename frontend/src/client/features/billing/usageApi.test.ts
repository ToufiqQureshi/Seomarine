import { describe, expect, it } from "vitest";
import { creditLevel, type BillingUsage } from "./usageApi";

function usage(over: Partial<BillingUsage>): BillingUsage {
  return {
    plan: "pro",
    unlimited: false,
    chargesCredits: true,
    balanceCredits: 5000,
    monthlyCredits: 10000,
    periodStart: "2026-10-01T00:00:00Z",
    periodEnd: null,
    spent: [],
    costs: [],
    ...over,
  };
}

describe("creditLevel", () => {
  it.each([
    ["plenty left", usage({}), "ok"],
    [
      "exactly a tenth left is still fine",
      usage({ balanceCredits: 1000 }),
      "ok",
    ],
    ["under a tenth left is low", usage({ balanceCredits: 999 }), "low"],
    ["nothing left", usage({ balanceCredits: 0 }), "out"],
    [
      "free plan without credits",
      usage({ plan: "free", balanceCredits: 0, monthlyCredits: 0 }),
      "out",
    ],
    [
      "staff workspace",
      usage({ unlimited: true, balanceCredits: 0 }),
      "not_charged",
    ],
    [
      "server without payments",
      usage({ chargesCredits: false, balanceCredits: 0 }),
      "not_charged",
    ],
  ] as const)("%s", (_name, input, want) => {
    expect(creditLevel(input)).toBe(want);
  });
});
