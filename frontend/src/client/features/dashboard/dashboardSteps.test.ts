import { describe, expect, it } from "vitest";
import type { DashboardActivation } from "@/client/features/dashboard/dashboardApi";
import { getStepStatus, setupSteps } from "./dashboardSteps";

const activation: DashboardActivation = {
  domain: null,
  hasMultipleProjects: false,
  hasTeammate: false,
  dismissedSteps: [],
  ga4: { connected: false, propertyDisplayName: null, cardDismissedAt: null },
  gsc: { connected: false, siteUrl: null },
  mcp: { authorizedAt: null, firstToolCallAt: null, cardDismissedAt: null },
  competitorClickedAt: null,
  keywordsClickedAt: null,
  hasAudit: false,
};

describe("dashboard setup step status", () => {
  it("marks product events, click-throughs, and manual milestones done", () => {
    expect(
      getStepStatus(
        { ...activation, competitorClickedAt: "now" },
        "competitor",
      ),
    ).toBe("done");
    expect(
      getStepStatus({ ...activation, keywordsClickedAt: "now" }, "keywords"),
    ).toBe("done");
    expect(getStepStatus({ ...activation, hasAudit: true }, "audit")).toBe(
      "done",
    );
    expect(
      getStepStatus(
        { ...activation, mcp: { ...activation.mcp, firstToolCallAt: "now" } },
        "mcp",
      ),
    ).toBe("done");
    expect(getStepStatus({ ...activation, hasTeammate: true }, "team")).toBe(
      "done",
    );
    expect(
      getStepStatus({ ...activation, hasMultipleProjects: true }, "project"),
    ).toBe("done");
  });

  it("keeps dismissals skipped, then incomplete steps todo", () => {
    expect(
      getStepStatus({ ...activation, dismissedSteps: ["audit"] }, "audit"),
    ).toBe("skipped");
    expect(
      getStepStatus(
        { ...activation, mcp: { ...activation.mcp, cardDismissedAt: "now" } },
        "mcp",
      ),
    ).toBe("skipped");
    expect(getStepStatus(activation, "audit")).toBe("todo");
  });

  it("keeps the checklist definition complete and unique", () => {
    expect(setupSteps.map(({ id }) => id)).toEqual([
      "competitor",
      "keywords",
      "audit",
      "mcp",
      "team",
      "project",
    ]);
  });
});
