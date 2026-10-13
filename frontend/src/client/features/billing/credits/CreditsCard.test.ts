import { createElement, type ReactNode } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";
import type { BillingUsage } from "@/client/features/billing/usageApi";
import { CreditsCard } from "./CreditsCard";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children }: { children: ReactNode }) =>
    createElement("a", null, children),
}));

const pro: BillingUsage = {
  plan: "pro",
  unlimited: false,
  chargesCredits: true,
  balanceCredits: 7420,
  monthlyCredits: 10000,
  periodStart: "2026-10-01T00:00:00Z",
  periodEnd: "2026-11-01T00:00:00Z",
  spent: [],
  costs: [],
};

function render(usage: BillingUsage, canManageBilling = true) {
  return renderToStaticMarkup(
    createElement(CreditsCard, { usage, canManageBilling }),
  );
}

describe("CreditsCard", () => {
  it("shows what is left, the monthly allowance and the refill date", () => {
    const html = render(pro);
    expect(html).toContain("7,420");
    expect(html).toContain("credits left");
    expect(html).toContain("adds 10,000 credits every month");
    expect(html).toContain("next refill 1 Nov");
    expect(html).not.toContain("running low");
  });

  it("warns when credits run low", () => {
    expect(render({ ...pro, balanceCredits: 300 })).toContain("running low");
  });

  it("says live data is paused when none are left", () => {
    expect(render({ ...pro, balanceCredits: 0 })).toContain("No credits left");
  });

  it("tells a Free workspace it has no credits", () => {
    const html = render({
      ...pro,
      plan: "free",
      balanceCredits: 0,
      monthlyCredits: 0,
      periodEnd: null,
    });
    expect(html).toContain("The Free plan has no credits");
  });

  it("hides the numbers where credits are not charged", () => {
    const html = render({ ...pro, unlimited: true });
    expect(html).toContain("not charged credits");
    expect(html).not.toContain("credits left");
  });

  it("tells members who cannot change the plan", () => {
    expect(render(pro, false)).toContain(
      "Only the organization owner can change the plan",
    );
  });
});
