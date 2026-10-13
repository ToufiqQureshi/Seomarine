import { afterEach, describe, expect, it, vi } from "vitest";
import {
  getDashboardActivation,
  getDashboardOverview,
  markDashboardStepClicked,
  setDashboardStepDismissed,
} from "./dashboardApi";

afterEach(() => vi.restoreAllMocks());

describe("dashboard API", () => {
  it("encodes project IDs and validates activation state", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        domain: null,
        ga4: {
          connected: false,
          propertyDisplayName: null,
          cardDismissedAt: null,
        },
        gsc: { connected: false, siteUrl: null },
        mcp: {
          authorizedAt: null,
          firstToolCallAt: null,
          cardDismissedAt: null,
        },
        competitorClickedAt: null,
        keywordsClickedAt: null,
        hasAudit: false,
        hasMultipleProjects: false,
        hasTeammate: false,
        dismissedSteps: [],
      }),
    );
    await expect(
      getDashboardActivation({ data: { projectId: "p/1" } }),
    ).resolves.toMatchObject({ hasAudit: false });
    expect(fetchMock.mock.calls[0]).toMatchObject([
      "/api/v1/projects/p%2F1/activation/get",
      { method: "POST", body: "{}" },
    ]);
    await expect(
      getDashboardActivation({ data: { projectId: "" } }),
    ).rejects.toThrow();
  });

  it("accepts empty overview cards and sends only supported activation changes", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValueOnce(Response.json({ audit: null, backlinks: null }))
      .mockResolvedValueOnce(Response.json({ ok: true }))
      .mockResolvedValueOnce(Response.json({ ok: true }));
    await expect(
      getDashboardOverview({ data: { projectId: "p1" } }),
    ).resolves.toEqual({ audit: null, backlinks: null });
    await markDashboardStepClicked({
      data: { projectId: "p1", step: "competitor" },
    });
    await setDashboardStepDismissed({
      data: { projectId: "p1", step: "audit", dismissed: true },
    });
    expect(fetchMock.mock.calls.slice(1)).toMatchObject([
      [
        "/api/v1/projects/p1/activation/click",
        { body: JSON.stringify({ step: "competitor" }) },
      ],
      [
        "/api/v1/projects/p1/activation/dismiss",
        { body: JSON.stringify({ step: "audit", dismissed: true }) },
      ],
    ]);
  });
});
