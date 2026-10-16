import { describe, expect, it } from "vitest";
import { buildContentRefreshQueue } from "@/client/features/search-performance/contentRefreshQueue";

describe("buildContentRefreshQueue", () => {
  it("groups pages with meaningful impressions in positions 5–20", () => {
    const queue = buildContentRefreshQueue([
      {
        query: "topic one",
        page: "/guide",
        clicks: 10,
        impressions: 100,
        position: 8,
      },
      {
        query: "topic two",
        page: "/guide",
        clicks: 4,
        impressions: 50,
        position: 12,
      },
      {
        query: "top result",
        page: "/top",
        clicks: 8,
        impressions: 400,
        position: 2,
      },
      {
        query: "low data",
        page: "/small",
        clicks: 1,
        impressions: 20,
        position: 9,
      },
    ]);

    expect(queue).toHaveLength(1);
    expect(queue[0]).toMatchObject({
      page: "/guide",
      clicks: 14,
      impressions: 150,
      averagePosition: 9.333333333333334,
    });
    expect(queue[0].queries.map((row) => row.query)).toEqual([
      "topic one",
      "topic two",
    ]);
  });

  it("applies the impression threshold to the grouped page total", () => {
    const queue = buildContentRefreshQueue([
      { query: "first", page: "/guide", clicks: 1, impressions: 30, position: 8 },
      { query: "second", page: "/guide", clicks: 2, impressions: 25, position: 12 },
    ]);
    expect(queue).toHaveLength(1);
    expect(queue[0]).toMatchObject({ page: "/guide", impressions: 55, clicks: 3 });
  });
});
