import type { AuditResultsData } from "@/client/features/audit/auditApi";

type AuditPage = AuditResultsData["pages"][number];
type AuditIssue = AuditResultsData["issues"][number];

const STOP_WORDS = new Set([
  "about",
  "and",
  "are",
  "for",
  "from",
  "how",
  "into",
  "more",
  "page",
  "the",
  "this",
  "with",
  "your",
]);

export function getOrphanLinkCandidates(
  orphanUrl: string,
  pages: AuditPage[],
  issues: AuditIssue[],
) {
  const orphan = pages.find(
    (page) => normalizeUrl(page.url) === normalizeUrl(orphanUrl),
  );
  const targetTokens = tokens(`${orphan?.title ?? ""} ${pathOf(orphanUrl)}`);
  if (targetTokens.size === 0) return [];
  const knownOrphans = new Set(
    issues
      .filter((issue) => issue.issueType === "orphan-page")
      .map((issue) => normalizeUrl(issue.pageUrl)),
  );

  return pages
    .filter(
      (page) =>
        page.isIndexable &&
        page.fetchClass === "ok" &&
        normalizeUrl(page.url) !== normalizeUrl(orphanUrl) &&
        !knownOrphans.has(normalizeUrl(page.url)),
    )
    .map((page) => {
      const sourceTokens = tokens(`${page.title ?? ""} ${pathOf(page.url)}`);
      const overlap = [...targetTokens].filter((token) =>
        sourceTokens.has(token),
      ).length;
      return { page, overlap };
    })
    .filter((candidate) => candidate.overlap > 0)
    .toSorted(
      (a, b) =>
        b.overlap - a.overlap ||
        b.page.internalLinkCount - a.page.internalLinkCount ||
        a.page.url.localeCompare(b.page.url),
    )
    .slice(0, 3)
    .map(({ page, overlap }) => ({
      sourceUrl: page.url,
      sourceTitle: page.title?.trim() || page.url,
      targetTitle: orphan?.title?.trim() || titleFromPath(orphanUrl),
      targetUrl: orphanUrl,
      matchedTerms: overlap,
    }));
}

function tokens(value: string) {
  return new Set(
    value
      .toLowerCase()
      .normalize("NFKD")
      .replace(/[^a-z0-9]+/g, " ")
      .split(/\s+/)
      .filter((token) => token.length > 2 && !STOP_WORDS.has(token)),
  );
}

function pathOf(rawUrl: string) {
  try {
    return decodeURIComponent(
      new URL(rawUrl).pathname.replace(/\.[a-z0-9]+$/i, ""),
    );
  } catch {
    return rawUrl;
  }
}

function titleFromPath(rawUrl: string) {
  const lastSegment = pathOf(rawUrl).split("/").filter(Boolean).at(-1);
  return lastSegment?.replace(/[-_]+/g, " ") || rawUrl;
}

function normalizeUrl(rawUrl: string) {
  try {
    const url = new URL(rawUrl);
    url.hash = "";
    if (url.pathname.length > 1) url.pathname = url.pathname.replace(/\/$/, "");
    return url.toString();
  } catch {
    return rawUrl;
  }
}
