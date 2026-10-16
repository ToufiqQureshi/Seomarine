import type {
  AuditHistory,
  AuditResultsData,
} from "@/client/features/audit/auditApi";

type AuditChange = {
  url: string;
  field:
    | "Title"
    | "Meta description"
    | "Canonical URL"
    | "Indexability"
    | "HTTP status";
  before: string;
  after: string;
};

export function latestComparablePair(history: AuditHistory) {
  const latestByStartUrl = new Map<string, AuditHistory[number]>();
  for (const audit of history) {
    if (audit.status !== "completed") continue;
    const key = normalizeUrl(audit.startUrl);
    const latest = latestByStartUrl.get(key);
    if (latest) return [latest, audit];
    latestByStartUrl.set(key, audit);
  }
  return [];
}

export function compareAuditResults(
  older: AuditResultsData,
  newer: AuditResultsData,
) {
  const oldPages = new Map(
    older.pages.map((page) => [normalizeUrl(page.url), page]),
  );
  const newPages = new Map(
    newer.pages.map((page) => [normalizeUrl(page.url), page]),
  );
  const changes: AuditChange[] = [];
  let addedPages = 0;
  let removedPages = 0;

  for (const [key, page] of newPages) {
    const previous = oldPages.get(key);
    if (!previous) {
      addedPages++;
      continue;
    }
    addChange(changes, page.url, "Title", previous.title, page.title);
    addChange(
      changes,
      page.url,
      "Meta description",
      previous.metaDescription,
      page.metaDescription,
    );
    addChange(
      changes,
      page.url,
      "Canonical URL",
      previous.canonicalUrl,
      page.canonicalUrl,
    );
    addChange(
      changes,
      page.url,
      "Indexability",
      previous.isIndexable ? "Indexable" : "Noindex",
      page.isIndexable ? "Indexable" : "Noindex",
    );
    addChange(
      changes,
      page.url,
      "HTTP status",
      previous.statusCode?.toString() ?? "Unknown",
      page.statusCode?.toString() ?? "Unknown",
    );
  }
  for (const key of oldPages.keys()) {
    if (!newPages.has(key)) removedPages++;
  }

  return {
    changes,
    addedPages,
    removedPages,
    oldIssueCount: older.issues.length,
    newIssueCount: newer.issues.length,
    issueDelta: newer.issues.length - older.issues.length,
  };
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

function addChange(
  changes: AuditChange[],
  url: string,
  field: AuditChange["field"],
  before: string | null,
  after: string | null,
) {
  if ((before ?? "") === (after ?? "")) return;
  changes.push({
    url,
    field,
    before: before || "Empty",
    after: after || "Empty",
  });
}
