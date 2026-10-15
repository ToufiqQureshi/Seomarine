import { describe, expect, it } from "vitest";
import type { AuditResultsData } from "@/client/features/audit/auditApi";
import { getOrphanLinkCandidates } from "@/client/features/audit/orphanLinkCandidates";

const basePage = {
  id: "id",
  statusCode: 200,
  fetchClass: "ok" as const,
  redirectUrl: null,
  title: null,
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
  wordCount: 100,
  contentHash: null,
  imagesTotal: 0,
  imagesMissingAlt: 0,
  images: [],
  internalLinkCount: 3,
  externalLinkCount: 0,
  hasStructuredData: false,
  hreflangTags: [],
  isIndexable: true,
  crawlDepth: 1,
  inSitemap: true,
  responseTimeMs: 40,
};

function page(url: string, title: string, internalLinkCount = 3) {
  return { ...basePage, id: url, url, title, internalLinkCount };
}

describe("getOrphanLinkCandidates", () => {
  it("suggests relevant indexable source pages and excludes other orphan pages", () => {
    const pages: AuditResultsData["pages"] = [
      page("https://example.com/orphan/seo-audit", "SEO audit guide"),
      page("https://example.com/guides/seo", "SEO guide", 10),
      page("https://example.com/blog/seo-tools", "SEO tools", 2),
      page("https://example.com/orphan/seo-tips", "SEO tips", 100),
      page("https://example.com/products", "Products", 20),
    ];
    const issues: AuditResultsData["issues"] = [
      {
        id: "orphan",
        pageId: null,
        pageUrl: "https://example.com/orphan/seo-tips",
        issueType: "orphan-page",
        severity: "warning",
        detailsJson: null,
      },
    ];

    const candidates = getOrphanLinkCandidates(
      "https://example.com/orphan/seo-audit",
      pages,
      issues,
    );
    expect(candidates.map((candidate) => candidate.sourceUrl)).toEqual([
      "https://example.com/guides/seo",
      "https://example.com/blog/seo-tools",
    ]);
  });

  it("returns no suggestions when there are no shared topic terms", () => {
    const candidates = getOrphanLinkCandidates(
      "https://example.com/orphan/insurance",
      [
        page("https://example.com/blog/gardening", "Gardening"),
        page("https://example.com/orphan/insurance", "Insurance"),
      ],
      [],
    );
    expect(candidates).toEqual([]);
  });
});
