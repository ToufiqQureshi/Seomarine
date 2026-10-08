import { describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/seomarineApi";
import {
  analyticsCountriesPage,
  analyticsSummaryQueryOptions,
  rangeForDays,
} from "./analyticsApi";

describe("rangeForDays", () => {
  it.each([
    {
      name: "7 days include today",
      days: 7 as const,
      now: "2026-10-07T12:00:00Z",
      range: { from: "2026-10-01", to: "2026-10-07" },
    },
    {
      name: "crosses a year boundary",
      days: 30 as const,
      now: "2026-01-10T00:00:00Z",
      range: { from: "2025-12-12", to: "2026-01-10" },
    },
    {
      name: "uses the UTC day, as the API does",
      days: 7 as const,
      now: "2026-03-01T23:30:00-05:00",
      range: { from: "2026-02-24", to: "2026-03-02" },
    },
  ])("$name", ({ days, now, range }) => {
    expect(rangeForDays(days, new Date(now))).toEqual(range);
  });
});

describe("analytics countries query", () => {
  it("requests a bounded page and validates country rows", async () => {
    const rows = [{ code: "IN", name: "India", visitors: 7, pct: 70 }];
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json(rows));
    await expect(
      analyticsCountriesPage(12, { from: "2026-10-01", to: "2026-10-07" }, 2),
    ).resolves.toEqual(rows);
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/analytics/12/countries?from=2026-10-01&to=2026-10-07&page=2&limit=10",
    );
  });
});

const summary = {
  visitors: 3,
  pageviews: 5,
  series: [{ date: "2026-10-07", visitors: 3, pageviews: 5 }],
  topPages: [{ path: "/", pageviews: 5 }],
  channels: [{ channel: "ai", visitors: 3 }],
  aiSources: [{ source: "chatgpt", visitors: 3 }],
  referrers: [{ host: "chatgpt.com", visitors: 3 }],
  devices: [{ device: "mobile", visitors: 3 }],
};

describe("analytics summary query", () => {
  const range = { from: "2026-10-01", to: "2026-10-07" };

  it("requests the project's range and returns the parsed summary", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json(summary));
    const data = await analyticsSummaryQueryOptions("p/1", range).queryFn();
    expect(data).toEqual(summary);
    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/projects/p%2F1/analytics/summary?from=2026-10-01&to=2026-10-07",
    );
  });

  it.each([
    {
      name: "an unknown channel",
      patch: { channels: [{ channel: "email", visitors: 1 }] },
    },
    { name: "a null list", patch: { referrers: null } },
    { name: "a negative count", patch: { visitors: -1 } },
    {
      name: "a malformed date",
      patch: { series: [{ date: "07/10/2026", visitors: 1, pageviews: 1 }] },
    },
  ])("rejects $name", async ({ patch }) => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({ ...summary, ...patch }),
    );
    await expect(
      analyticsSummaryQueryOptions("p1", range).queryFn(),
    ).rejects.toBeInstanceOf(ApiError);
  });
});
