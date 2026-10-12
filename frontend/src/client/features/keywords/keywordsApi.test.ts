import { afterEach, describe, expect, it, vi } from "vitest";
import { prewarmSerpLocations, searchSerpLocations } from "./keywordsApi";

afterEach(() => {
  vi.restoreAllMocks();
});

describe("SERP location API", () => {
  it("posts the query and country to the Go search route", async () => {
    const row = {
      locationCode: 1,
      locationName: "Portland,Maine,United States",
      locationType: "City",
      displayLabel: "Portland, Maine",
    };
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json([row]));

    const rows = await searchSerpLocations({
      countryCode: "us",
      query: "port",
    });

    expect(rows).toEqual([row]);
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/serp-locations/search");
    expect(fetchMock.mock.calls[0][1]?.body).toBe(
      JSON.stringify({ countryCode: "us", query: "port" }),
    );
  });

  it("rejects a malformed location row instead of rendering it", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json([{ locationCode: "1" }]),
    );
    await expect(
      searchSerpLocations({ countryCode: "us", query: "port" }),
    ).rejects.toThrow();
  });

  it("posts only the country to the prewarm route", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ warmed: true }));

    await expect(prewarmSerpLocations({ countryCode: "in" })).resolves.toEqual({
      warmed: true,
    });
    expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/serp-locations/prewarm");
    expect(fetchMock.mock.calls[0][1]?.body).toBe(
      JSON.stringify({ countryCode: "in" }),
    );
  });
});
