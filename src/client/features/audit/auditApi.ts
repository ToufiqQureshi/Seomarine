import { z } from "zod";
import { apiRequest } from "@/client/lib/seomarineApi";
import { PAGE_FETCH_CLASSES } from "@/shared/audit-fetch-class";
import { LIGHTHOUSE_CATEGORIES } from "@/shared/lighthouse";

/**
 * Client for the Go site-audit API (`/api/v1/projects/{projectId}/audit/*`).
 *
 * These are RESPONSE schemas: the Go server owns request validation, and the
 * shapes here mirror `backend/internal/audit/repository.go` field by field.
 * `auditApi.test.ts`-style contracts live with the Go handler tests; a drift in
 * either direction fails the Zod parse and surfaces as an "invalid response"
 * error rather than a half-rendered audit page.
 */

const nullableNumber = z.number().nullable();
const nullableString = z.string().nullable();

const fetchClassSchema = z.enum(PAGE_FETCH_CLASSES);
const severitySchema = z.enum(["critical", "warning", "info"]);
const lighthouseStrategySchema = z.enum(["auto", "none"]);

const auditConfigSchema = z.object({
  maxPages: z.number(),
  lighthouseStrategy: lighthouseStrategySchema,
  renderJavaScript: z.boolean().default(false),
  sitePlatform: z.literal("shopify").optional(),
  crawlerCredentialId: z.string().optional(),
});

const auditStatusSchema = z.object({
  id: z.string(),
  startUrl: z.string(),
  status: z.enum(["running", "completed", "failed"]),
  pagesCrawled: z.number(),
  pagesTotal: z.number(),
  lighthouseTotal: z.number(),
  lighthouseCompleted: z.number(),
  lighthouseFailed: z.number(),
  currentPhase: nullableString,
  errorCode: nullableString,
  startedAt: z.string(),
  completedAt: nullableString,
});

const auditPageSchema = z.object({
  id: z.string(),
  url: z.string(),
  statusCode: nullableNumber,
  fetchClass: fetchClassSchema,
  redirectUrl: nullableString,
  title: nullableString,
  metaDescription: nullableString,
  canonicalUrl: nullableString,
  robotsMeta: nullableString,
  xRobotsTag: nullableString,
  headerCanonicalUrl: nullableString,
  ogTitle: nullableString,
  ogDescription: nullableString,
  ogImage: nullableString,
  h1Count: z.number(),
  h2Count: z.number(),
  h3Count: z.number(),
  h4Count: z.number(),
  h5Count: z.number(),
  h6Count: z.number(),
  headingOrder: z.array(z.number()),
  wordCount: z.number(),
  contentHash: nullableString,
  imagesTotal: z.number(),
  imagesMissingAlt: z.number(),
  images: z.array(z.object({ src: z.string(), alt: z.string() })),
  internalLinkCount: z.number(),
  externalLinkCount: z.number(),
  hasStructuredData: z.boolean(),
  hreflangTags: z.array(z.string()),
  isIndexable: z.boolean(),
  crawlDepth: nullableNumber,
  inSitemap: z.boolean(),
  responseTimeMs: nullableNumber,
});

const auditIssueSchema = z
  .object({
    id: z.string(),
    pageId: nullableString,
    pageUrl: z.string(),
    issueType: z.string(),
    severity: severitySchema,
    details: z.record(z.string(), z.unknown()).optional(),
  })
  .transform(({ details, ...issue }) => ({
    ...issue,
    detailsJson: details ? JSON.stringify(details) : null,
  }));

const lighthouseResultSchema = z.object({
  id: z.string(),
  pageId: z.string(),
  url: z.string(),
  strategy: z.enum(["mobile", "desktop"]),
  performanceScore: nullableNumber,
  accessibilityScore: nullableNumber,
  bestPracticesScore: nullableNumber,
  seoScore: nullableNumber,
  lcpMs: nullableNumber,
  cls: nullableNumber,
  inpMs: nullableNumber,
  ttfbMs: nullableNumber,
  errorMessage: nullableString.optional().transform((value) => value ?? null),
  hasPayload: z.boolean(),
  payloadSizeBytes: nullableNumber
    .optional()
    .transform((value) => value ?? null),
});

export const auditResultsSchema = z.object({
  audit: z.object({
    id: z.string(),
    startUrl: z.string(),
    status: z.enum(["running", "completed", "failed"]),
    pagesCrawled: z.number(),
    pagesTotal: z.number(),
    startedAt: z.string(),
    completedAt: nullableString,
    config: auditConfigSchema,
  }),
  pages: z.array(auditPageSchema),
  lighthouse: z.array(lighthouseResultSchema),
  issues: z.array(auditIssueSchema),
});

export type AuditResultsData = z.infer<typeof auditResultsSchema>;

const auditHistoryItemSchema = z.object({
  id: z.string(),
  startUrl: z.string(),
  status: z.enum(["running", "completed", "failed"]),
  pagesCrawled: z.number(),
  pagesTotal: z.number(),
  ranLighthouse: z.boolean(),
  startedAt: z.string(),
  completedAt: nullableString,
});

export type AuditHistory = z.infer<typeof auditHistoryItemSchema>[];

const crawlProgressSchema = z.array(
  z.object({
    url: z.string(),
    statusCode: z.number(),
    title: z.string(),
    crawledAt: z.number(),
  }),
);

const lighthouseCategorySchema = z.enum(LIGHTHOUSE_CATEGORIES);

const lighthouseMetricSchema = z.object({
  score: nullableNumber,
  displayValue: nullableString,
  numericValue: nullableNumber,
});

const lighthouseIssuesSchema = z.object({
  id: z.string(),
  finalUrl: z.string(),
  strategy: z.enum(["mobile", "desktop"]),
  createdAt: z.string(),
  hasIssueDetails: z.boolean(),
  scores: z
    .object({
      performance: nullableNumber,
      accessibility: nullableNumber,
      "best-practices": nullableNumber,
      seo: nullableNumber,
    })
    .nullable(),
  metrics: z
    .object({
      firstContentfulPaint: lighthouseMetricSchema,
      largestContentfulPaint: lighthouseMetricSchema,
      totalBlockingTime: lighthouseMetricSchema,
      cumulativeLayoutShift: lighthouseMetricSchema,
      speedIndex: lighthouseMetricSchema,
      timeToInteractive: lighthouseMetricSchema,
      interactionToNextPaint: lighthouseMetricSchema,
      serverResponseTime: lighthouseMetricSchema,
    })
    .nullable(),
  issues: z.array(
    z.object({
      category: lighthouseCategorySchema,
      auditKey: z.string(),
      title: z.string(),
      description: z.string(),
      score: nullableNumber,
      scoreDisplayMode: nullableString,
      displayValue: nullableString,
      impactMs: nullableNumber,
      impactBytes: nullableNumber,
      severity: severitySchema,
      items: z.array(z.string()),
    }),
  ),
});

export type LighthouseIssuesData = z.infer<typeof lighthouseIssuesSchema>;

const lighthouseExportSchema = z.object({
  filename: z.string(),
  content: z.string(),
});

const startAuditSchema = z.object({ auditId: z.string() });
const deleteAuditSchema = z.object({ success: z.boolean() });
const capabilitiesSchema = z.object({ canRenderJavaScript: z.boolean() });

function auditPath(projectId: string, endpoint: string) {
  return `/api/v1/projects/${encodeURIComponent(projectId)}/audit/${endpoint}`;
}

/** Starts an audit and returns its id. */
export function startAudit(input: {
  data: {
    projectId: string;
    startUrl: string;
    maxPages: number;
    lighthouseStrategy: "auto" | "none";
    renderJavaScript: boolean;
  };
}) {
  return apiRequest(
    auditPath(input.data.projectId, "start"),
    startAuditSchema,
    "POST",
    input.data,
  );
}

/** Returns an audit's status. */
export function getAuditStatus(input: {
  data: { projectId: string; auditId: string };
}) {
  return apiRequest(
    auditPath(input.data.projectId, "status"),
    auditStatusSchema,
    "POST",
    { auditId: input.data.auditId },
  );
}

/** Returns an audit's pages, issues and Lighthouse results. */
export function getAuditResults(input: {
  data: { projectId: string; auditId: string };
}) {
  return apiRequest(
    auditPath(input.data.projectId, "results"),
    auditResultsSchema,
    "POST",
    { auditId: input.data.auditId },
  );
}

/** Lists a project's audits, newest first. */
export function getAuditHistory(input: { data: { projectId: string } }) {
  return apiRequest(
    auditPath(input.data.projectId, "history"),
    z.array(auditHistoryItemSchema),
    "POST",
    {},
  );
}

/** Returns the live crawl feed of a running audit. */
export function getCrawlProgress(input: {
  data: { projectId: string; auditId: string };
}) {
  return apiRequest(
    auditPath(input.data.projectId, "progress"),
    crawlProgressSchema,
    "POST",
    { auditId: input.data.auditId },
  );
}

/** Stops and deletes an audit. */
export function deleteAudit(input: {
  data: { projectId: string; auditId: string };
}) {
  return apiRequest(
    auditPath(input.data.projectId, "delete"),
    deleteAuditSchema,
    "POST",
    { auditId: input.data.auditId },
  );
}

/** Reports which audit features this deployment supports. */
export function getAuditCapabilities(input: { data: { projectId: string } }) {
  return apiRequest(
    auditPath(input.data.projectId, "capabilities"),
    capabilitiesSchema,
    "POST",
    {},
  );
}

/** Returns the stored issues of one Lighthouse result. */
export function getAuditLighthouseIssues(input: {
  data: { projectId: string; resultId: string };
}) {
  return apiRequest(
    auditPath(input.data.projectId, "lighthouse/issues"),
    lighthouseIssuesSchema,
    "POST",
    { resultId: input.data.resultId },
  );
}

/** Builds a downloadable export of one Lighthouse result. */
export function exportAuditLighthouseIssues(input: {
  data: {
    projectId: string;
    resultId: string;
    mode: "full" | "issues" | "category";
    category?: (typeof LIGHTHOUSE_CATEGORIES)[number];
  };
}) {
  const { projectId, ...body } = input.data;
  return apiRequest(
    auditPath(projectId, "lighthouse/export"),
    lighthouseExportSchema,
    "POST",
    body,
  );
}
