import { createElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { BillingUsage } from "@/client/features/billing/usageApi";
import { CreditCostHint } from "./CreditCostHint";

const usageState: { data: BillingUsage | undefined } = { data: undefined };

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) =>
    createElement("a", null, children),
}));

vi.mock("@/client/features/billing/usageApi", async (importOriginal) => ({
  ...(await importOriginal<object>()),
  useBillingUsage: () => usageState,
}));

const proUsage: BillingUsage = {
  plan: "pro",
  unlimited: false,
  chargesCredits: true,
  balanceCredits: 7420,
  monthlyCredits: 10000,
  periodStart: "2026-10-01T00:00:00Z",
  periodEnd: null,
  spent: [],
  costs: [{ feature: "keyword_research", unit: "keyword search", credits: 26 }],
};

function render() {
  return renderToStaticMarkup(
    createElement(CreditCostHint, { feature: "keyword_research" }),
  );
}

describe("CreditCostHint", () => {
  beforeEach(() => {
    usageState.data = proUsage;
  });

  it("shows the typical cost and what is left", () => {
    expect(render()).toContain(
      "About 26 credits per keyword search · 7,420 left",
    );
  });

  it("says when no credits are left", () => {
    usageState.data = { ...proUsage, balanceCredits: 0 };
    expect(render()).toContain("No credits left.");
  });

  it("shows nothing while loading, so the action is never blocked", () => {
    usageState.data = undefined;
    expect(render()).toBe("");
  });

  it("shows nothing where credits are not charged", () => {
    usageState.data = { ...proUsage, unlimited: true };
    expect(render()).toBe("");
  });

  it("still shows the balance for a feature without a listed cost", () => {
    usageState.data = { ...proUsage, costs: [] };
    expect(render()).toContain("7,420 left");
    expect(render()).not.toContain("About");
  });
});
