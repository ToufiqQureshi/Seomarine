import { afterEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/uplumoApi";
import { runLocalGrid } from "./localSeoApi";

afterEach(() => vi.restoreAllMocks());

describe("runLocalGrid", () => {
  it("sends bounded grid options to the project endpoint", async () => {
    const response = {
      grid: [
        {
          row: 0,
          col: 0,
          latitude: 19,
          longitude: 72,
          rank: 2,
          resultsCount: 5,
        },
      ],
      summary: {
        pointsSearched: 1,
        pointsFound: 1,
        averageRank: 2,
        top3Count: 1,
        top10Count: 1,
        failedPoints: 0,
      },
      estimatedRequests: 1,
    };
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json(response));
    await runLocalGrid({
      projectId: "project/1",
      keyword: "dentist",
      target: { name: "Harbor Dental" },
      center: { latitude: 19, longitude: 72 },
      gridSize: 3,
      spacingKm: 1,
      device: "mobile",
      languageCode: "en",
    });
    const [path, init] = fetchMock.mock.calls[0];
    expect(path).toBe("/api/v1/projects/project%2F1/local-seo/rank-grid");
    expect(init?.method).toBe("POST");
    const requestBody = init?.body;
    expect(typeof requestBody).toBe("string");
    if (typeof requestBody !== "string")
      throw new Error("request body missing");
    expect(JSON.parse(requestBody)).toMatchObject({
      keyword: "dentist",
      gridSize: 3,
      device: "mobile",
    });
  });

  it("surfaces upstream failures through the shared API error contract", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json(
        { error: { code: "upstream_error", message: "Maps lookup failed" } },
        { status: 502 },
      ),
    );
    await expect(
      runLocalGrid({
        projectId: "p1",
        keyword: "dentist",
        target: { name: "Clinic" },
        center: { latitude: 0, longitude: 0 },
        gridSize: 3,
        spacingKm: 1,
        device: "mobile",
        languageCode: "en",
      }),
    ).rejects.toBeInstanceOf(ApiError);
  });
});
