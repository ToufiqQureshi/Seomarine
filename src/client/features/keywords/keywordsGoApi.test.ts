import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/client/lib/seomarineApi";
import {
  deleteSavedKeywordTag,
  exportSavedKeywords,
  getKeywordResearch,
  getSavedKeywords,
  getSerpAnalysis,
  refreshSavedKeywordMetrics,
  removeSavedKeywords,
  researchKeywords,
  saveKeywords,
  updateSavedKeywordTag,
  updateSavedKeywordTags,
} from "./keywordsApi";

// The config's restoreMocks drops module-level spies before every test, so
// the fetch spy is (re)created in beforeEach and every test sees it.
let fetchSpy: ReturnType<typeof vi.spyOn>;

beforeEach(() => {
  fetchSpy = vi.spyOn(globalThis, "fetch").mockResolvedValue(Response.json({}));
});

function reply(value: unknown, status = 200) {
  fetchSpy.mockResolvedValue(Response.json(value, { status }));
}

const projectPath = "/api/v1/projects/p1";

describe("keyword research API", () => {
  it("posts the seeds to the Go research route", async () => {
    reply({
      rows: [
        {
          keyword: "seo",
          searchVolume: 1200,
          trend: [],
          keywordDifficulty: 42,
          cpc: 1.5,
          competition: 0.3,
          intent: "informational",
        },
      ],
      source: "blended",
      usedFallback: false,
    });

    const result = await researchKeywords({
      projectId: "p1",
      keywords: ["seo"],
      mode: "auto",
      resultLimit: 150,
    });

    expect(result.rows).toHaveLength(1);
    expect(fetchSpy.mock.calls[0][0]).toBe(`${projectPath}/keywords/research`);
    expect(fetchSpy.mock.calls[0][1]?.body).toBe(
      JSON.stringify({
        projectId: "p1",
        keywords: ["seo"],
        mode: "auto",
        resultLimit: 150,
      }),
    );
  });

  it("rejects a research row without an intent", async () => {
    reply({ rows: [{ keyword: "seo" }], source: "auto", usedFallback: false });
    await expect(
      researchKeywords({ projectId: "p1", keywords: ["seo"] }),
    ).rejects.toThrow(ApiError);
  });

  it("serves the Playwright fixture instead of a billed call", async () => {
    vi.stubEnv("VITE_E2E_KEYWORD_FIXTURES", "1");
    try {
      const result = await getKeywordResearch({
        data: { projectId: "p1", keywords: ["keyword research"] },
      });
      expect(result.rows.length).toBeGreaterThan(0);
      expect(fetchSpy).not.toHaveBeenCalled();
    } finally {
      vi.unstubAllEnvs();
    }
  });
});

describe("SERP analysis API", () => {
  it("posts the keyword, market and depth", async () => {
    reply({ requestedKeyword: "seo", items: [], depth: 20 });
    await getSerpAnalysis({
      projectId: "p1",
      keyword: "seo",
      locationCode: 2840,
      depth: 20,
    });
    expect(fetchSpy.mock.calls[0][0]).toBe(`${projectPath}/keywords/serp`);
    expect(fetchSpy.mock.calls[0][1]?.body).toBe(
      JSON.stringify({
        projectId: "p1",
        keyword: "seo",
        locationCode: 2840,
        depth: 20,
      }),
    );
  });

  it("surfaces a billed-route refusal as ApiError", async () => {
    reply({ error: { code: "payment_required", message: "Upgrade." } }, 402);
    await expect(
      getSerpAnalysis({ projectId: "p1", keyword: "seo" }),
    ).rejects.toMatchObject({ status: 402, code: "payment_required" });
  });
});

describe("saved keywords API", () => {
  it("posts the list filters to the saved/list route", async () => {
    reply({ rows: [], totalCount: 0, tags: [] });
    await getSavedKeywords({ projectId: "p1", page: 2, pageSize: 50 });
    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${projectPath}/keywords/saved/list`,
    );
    expect(fetchSpy.mock.calls[0][1]?.body).toBe(
      JSON.stringify({ projectId: "p1", page: 2, pageSize: 50 }),
    );
  });

  it("keeps the saved list shape", async () => {
    reply({
      rows: [
        {
          id: "k1",
          projectId: "p1",
          keyword: "seo",
          locationCode: 2840,
          languageCode: "en",
          createdAt: "2026-10-01T00:00:00Z",
          searchVolume: 10,
          cpc: null,
          competition: null,
          keywordDifficulty: null,
          intent: "commercial",
          monthlySearches: [],
          fetchedAt: null,
          tags: [
            {
              id: "t1",
              name: " Brand",
              normalizedName: "brand",
              color: "amber",
            },
          ],
        },
      ],
      totalCount: 1,
      tags: [
        {
          id: "t1",
          name: "Brand",
          normalizedName: "brand",
          color: "amber",
          keywordCount: 1,
        },
      ],
    });

    const result = await getSavedKeywords({ projectId: "p1" });
    expect(result.rows[0]?.tags[0]?.name).toBe(" Brand");
    expect(result.tags[0]?.keywordCount).toBe(1);
  });

  it("posts a bulk remove with the ids", async () => {
    reply({ success: true, deletedCount: 2 });
    await expect(
      removeSavedKeywords({ projectId: "p1", savedKeywordIds: ["a", "b"] }),
    ).resolves.toEqual({ success: true, deletedCount: 2 });
    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${projectPath}/keywords/saved/remove`,
    );
  });

  it("posts the export filters without paging", async () => {
    reply({ rows: [] });
    await expect(
      exportSavedKeywords({ projectId: "p1", sort: "cpc", order: "asc" }),
    ).resolves.toEqual({ rows: [] });
    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${projectPath}/keywords/saved/export`,
    );
  });
});

describe("saved keyword tag APIs", () => {
  it("posts an assignment edit", async () => {
    reply({
      success: true,
      taggedCount: 3,
      addedTags: [],
      removedTagIds: ["t1"],
      removedAssignments: 3,
    });
    await expect(
      updateSavedKeywordTags({
        projectId: "p1",
        savedKeywordIds: ["a", "b"],
        removeTagIds: ["t1"],
      }),
    ).resolves.toMatchObject({ taggedCount: 3, removedAssignments: 3 });
    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${projectPath}/keywords/saved/tags/assign`,
    );
  });

  it("surfaces a 409 tag_in_use through unmodified", async () => {
    reply(
      {
        error: { code: "tag_in_use", message: "Still in use." },
        assignmentCount: 4,
      },
      409,
    );
    await expect(
      deleteSavedKeywordTag({ projectId: "p1", tagId: "t1" }),
    ).rejects.toMatchObject({ status: 409, code: "tag_in_use" });
  });

  it("posts a tag rename", async () => {
    reply({
      success: true,
      tag: { id: "t1", name: "Brand", normalizedName: "brand", color: null },
    });
    await updateSavedKeywordTag({
      projectId: "p1",
      tagId: "t1",
      name: "Brand",
    });
    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${projectPath}/keywords/saved/tags/update`,
    );
  });
});

describe("saved save and refresh APIs", () => {
  it("posts the keywords with metrics", async () => {
    reply({ success: true, savedKeywordIds: ["k1"] });
    await expect(
      saveKeywords({
        projectId: "p1",
        keywords: ["seo"],
        metrics: [{ keyword: "seo", searchVolume: 10 }],
      }),
    ).resolves.toEqual({ success: true, savedKeywordIds: ["k1"] });
    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${projectPath}/keywords/saved/save`,
    );
  });

  it("posts the refresh route", async () => {
    reply({ updated: 7 });
    await expect(
      refreshSavedKeywordMetrics({ projectId: "p1" }),
    ).resolves.toEqual({ updated: 7 });
    expect(fetchSpy.mock.calls[0][0]).toBe(
      `${projectPath}/keywords/saved/refresh`,
    );
  });
});
