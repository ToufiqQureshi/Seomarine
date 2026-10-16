import { describe, expect, it } from "vitest";
import { compareAuditResults, latestComparablePair } from "@/client/features/audit/compare";
import type { AuditResultsData } from "@/client/features/audit/auditApi";

function results(pages: AuditResultsData["pages"]): AuditResultsData {
  return {
    audit: {
      id: "audit",
      startUrl: "https://example.com",
      status: "completed",
      pagesCrawled: pages.length,
      pagesTotal: pages.length,
      startedAt: "2026-01-01T00:00:00.000Z",
      completedAt: "2026-01-01T00:01:00.000Z",
      config: {
        maxPages: 100,
        lighthouseStrategy: "none",
        renderJavaScript: false,
      },
    },
    pages,
    issues: [],
    lighthouse: [],
  };
}

function page(url: string, title: string) {
  return {
    id: url,
    url,
    statusCode: 200,
    fetchClass: "ok" as const,
    redirectUrl: null,
    title,
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
    headingOrder: [],
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
    inSitemap: true,
    responseTimeMs: 20,
  };
}

describe("compareAuditResults", () => {
  it("matches trailing-slash URL variants and reports page and issue deltas", () => {
    const before = results([page("https://example.com/about/", "About")]);
    const after = results([
      page("https://example.com/about", "About us"),
      page("https://example.com/contact", "Contact"),
    ]);
    after.issues = [
      {
        id: "issue-1",
        pageId: null,
        pageUrl: "https://example.com/contact",
        issueType: "missing-title",
        severity: "warning",
        detailsJson: null,
      },
    ];

    const diff = compareAuditResults(before, after);
    expect(diff.changes).toEqual([
      {
        url: "https://example.com/about",
        field: "Title",
        before: "About",
        after: "About us",
      },
    ]);
    expect(diff.addedPages).toBe(1);
    expect(diff.removedPages).toBe(0);
    expect(diff.issueDelta).toBe(1);
  });

  it("counts pages missing from the latest crawl without labeling them as removed content", () => {
    const diff = compareAuditResults(
      results([page("https://example.com/old", "Old")]),
      results([]),
    );
    expect(diff.removedPages).toBe(1);
    expect(diff.changes).toEqual([]);
  });
  it("finds the latest matching pair when a newer crawl used another start URL", () => {
    const item = (id: string, startUrl: string) => ({
      id,
      startUrl,
      status: "completed" as const,
      pagesCrawled: 1,
      pagesTotal: 1,
      ranLighthouse: false,
      startedAt: "2026-01-01T00:00:00.000Z",
      completedAt: "2026-01-01T00:01:00.000Z",
    });
    const pair = latestComparablePair([
      item("latest-other", "https://other.example/"),
      item("newer-example", "https://example.com/"),
      item("older-example", "https://example.com"),
    ]);
    expect(pair.map((audit) => audit.id)).toEqual([
      "newer-example",
      "older-example",
    ]);
  });

});
