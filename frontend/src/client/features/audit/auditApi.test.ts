import { afterEach, describe, expect, it, vi } from "vitest";
import {
  exportAuditLighthouseIssues,
  getAuditLighthouseIssues,
  getAuditResults,
} from "./auditApi";

const page = {
  id: "page-1",
  url: "https://example.com/",
  statusCode: 200,
  fetchClass: "ok",
  redirectUrl: null,
  title: "Home",
  metaDescription: null,
  canonicalUrl: null,
  robotsMeta: null,
  xRobotsTag: null,
  headerCanonicalUrl: null,
  ogTitle: null,
  ogDescription: null,
  ogImage: null,
  h1Count: 1,
  h2Count: 0,
  h3Count: 0,
  h4Count: 0,
  h5Count: 0,
  h6Count: 0,
  headingOrder: [1],
  wordCount: 10,
  contentHash: null,
  imagesTotal: 0,
  imagesMissingAlt: 0,
  images: [],
  internalLinkCount: 0,
  externalLinkCount: 0,
  hasStructuredData: false,
  hreflangTags: [],
  isIndexable: true,
  crawlDepth: 0,
  inSitemap: false,
  responseTimeMs: 120,
};

const lighthouse = {
  id: "lh-1",
  pageId: "page-1",
  url: "https://example.com/",
  strategy: "mobile",
  performanceScore: 90,
  accessibilityScore: null,
  bestPracticesScore: null,
  seoScore: null,
  lcpMs: null,
  cls: null,
  inpMs: null,
  ttfbMs: null,
  hasPayload: true,
};

function resultsBody(overrides: Record<string, unknown>) {
  return {
    audit: {
      id: "audit-1",
      startUrl: "https://example.com/",
      status: "completed",
      pagesCrawled: 1,
      pagesTotal: 1,
      startedAt: "2026-10-09T10:00:00.000Z",
      completedAt: null,
      config: { maxPages: 50, lighthouseStrategy: "auto" },
    },
    pages: [page],
    lighthouse: [lighthouse],
    issues: [],
    ...overrides,
  };
}

afterEach(() => {
  vi.restoreAllMocks();
});

describe("getAuditResults", () => {
  it("turns the Go issue details object into the detailsJson string the UI reads", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json(
        resultsBody({
          issues: [
            {
              id: "i1",
              pageId: "page-1",
              pageUrl: "https://example.com/",
              issueType: "missing_title",
              severity: "warning",
              details: { length: 3 },
            },
            {
              id: "i2",
              pageId: null,
              pageUrl: "https://example.com/",
              issueType: "duplicate_title",
              severity: "info",
            },
          ],
        }),
      ),
    );

    const results = await getAuditResults({
      data: { projectId: "p1", auditId: "audit-1" },
    });

    expect(results.issues[0].detailsJson).toBe('{"length":3}');
    expect(results.issues[1].detailsJson).toBeNull();
    expect(results.audit.config.renderJavaScript).toBe(false);
  });

  it("defaults omitted Lighthouse fields to null and keeps hasPayload", async () => {
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json(
        resultsBody({
          lighthouse: [
            { ...lighthouse, hasPayload: true, payloadSizeBytes: 2048 },
            { ...lighthouse, id: "lh-2", hasPayload: false, errorMessage: "x" },
          ],
        }),
      ),
    );

    const { lighthouse: rows } = await getAuditResults({
      data: { projectId: "p1", auditId: "audit-1" },
    });

    expect(rows[0]).toMatchObject({
      hasPayload: true,
      errorMessage: null,
      payloadSizeBytes: 2048,
    });
    expect(rows[1]).toMatchObject({
      hasPayload: false,
      errorMessage: "x",
      payloadSizeBytes: null,
    });
  });

  it("rejects a response without hasPayload instead of rendering half a row", async () => {
    const { hasPayload: _omitted, ...withoutHasPayload } = lighthouse;
    vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json(resultsBody({ lighthouse: [withoutHasPayload] })),
    );

    await expect(
      getAuditResults({ data: { projectId: "p1", auditId: "audit-1" } }),
    ).rejects.toThrow();
  });
});

describe("Lighthouse endpoints", () => {
  it("posts the result id to the project-scoped issues route", async () => {
    const fetchMock = vi.spyOn(globalThis, "fetch").mockResolvedValue(
      Response.json({
        id: "lh-1",
        finalUrl: "https://example.com/",
        strategy: "mobile",
        createdAt: "2026-10-09T10:00:00.000Z",
        hasIssueDetails: false,
        scores: null,
        metrics: null,
        issues: [],
      }),
    );

    await getAuditLighthouseIssues({
      data: { projectId: "p 1", resultId: "lh-1" },
    });

    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/projects/p%201/audit/lighthouse/issues",
    );
    expect(fetchMock.mock.calls[0][1]?.body).toBe(
      JSON.stringify({ resultId: "lh-1" }),
    );
  });

  it("sends the export mode and category but not the project id in the body", async () => {
    const fetchMock = vi
      .spyOn(globalThis, "fetch")
      .mockResolvedValue(Response.json({ filename: "a.json", content: "{}" }));

    await exportAuditLighthouseIssues({
      data: {
        projectId: "p1",
        resultId: "lh-1",
        mode: "category",
        category: "seo",
      },
    });

    expect(fetchMock.mock.calls[0][0]).toBe(
      "/api/v1/projects/p1/audit/lighthouse/export",
    );
    expect(fetchMock.mock.calls[0][1]?.body).toBe(
      JSON.stringify({ resultId: "lh-1", mode: "category", category: "seo" }),
    );
  });
});
