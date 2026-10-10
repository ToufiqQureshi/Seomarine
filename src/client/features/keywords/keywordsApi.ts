import { z } from "zod";
import { apiRequest } from "@/client/lib/seomarineApi";
import { TAG_COLOR_KEYS } from "@/shared/tag-colors";
import type { MonthlySearch, SavedKeywordTag } from "@/types/keywords";

const serpLocationSchema = z.object({
  locationCode: z.number(),
  locationName: z.string(),
  locationType: z.string(),
  displayLabel: z.string(),
});

const serpLocationsSchema = z.array(serpLocationSchema);
const prewarmSchema = z.object({ warmed: z.boolean() });

export type SerpLocationResult = z.infer<typeof serpLocationSchema>;

export function searchSerpLocations(input: {
  countryCode: string;
  query: string;
}) {
  return apiRequest(
    "/api/v1/serp-locations/search",
    serpLocationsSchema,
    "POST",
    input,
  );
}

export function prewarmSerpLocations(input: { countryCode: string }) {
  return apiRequest(
    "/api/v1/serp-locations/prewarm",
    prewarmSchema,
    "POST",
    input,
  );
}

/* ------------------------------------------------------------------ */
/*  Go keyword API (`/api/v1/projects/{projectId}/keywords/...`).      */
/*  Request validation happens on the Go server; the schemas here are  */
/*  RESPONSE contracts mirroring `backend/internal/keywords` structs.  */
/* ------------------------------------------------------------------ */

/** Path of a keyword route under `/api/v1/projects/{projectId}/`. */
type KeywordRoutePath =
  | "keywords/research"
  | "keywords/serp"
  | "keywords/saved/refresh"
  | "keywords/saved/save"
  | "keywords/saved/list"
  | "keywords/saved/export"
  | "keywords/saved/remove"
  | "keywords/saved/tags/assign"
  | "keywords/saved/tags/update"
  | "keywords/saved/tags/delete";

function postKeyword<T>(
  projectId: string,
  route: KeywordRoutePath,
  schema: z.ZodType<T>,
  body: Record<string, unknown>,
): Promise<T> {
  return apiRequest(
    `/api/v1/projects/${encodeURIComponent(projectId)}/${route}`,
    schema,
    "POST",
    body,
  );
}

const monthlySearchSchema = z.object({
  year: z.number().int().positive(),
  month: z.number().int().min(1).max(12),
  searchVolume: z.number().int().nonnegative(),
}) satisfies z.ZodType<MonthlySearch>;

const tagSchema = z.object({
  id: z.string(),
  name: z.string(),
  normalizedName: z.string(),
  color: z.enum(TAG_COLOR_KEYS).nullable(),
}) satisfies z.ZodType<SavedKeywordTag>;

const researchRowSchema = z.object({
  keyword: z.string(),
  searchVolume: z.number().nullable(),
  trend: z.array(monthlySearchSchema),
  keywordDifficulty: z.number().nullable(),
  cpc: z.number().nullable(),
  competition: z.number().nullable(),
  intent: z.enum([
    "informational",
    "commercial",
    "transactional",
    "navigational",
    "unknown",
  ]),
});

const researchResultSchema = z.object({
  rows: z.array(researchRowSchema),
  source: z.string(),
  usedFallback: z.boolean(),
});

export function researchKeywords(input: {
  projectId: string;
  keywords: string[];
  locationCode?: number;
  languageCode?: string;
  locationName?: string;
  resultLimit?: 150 | 300 | 500;
  mode?: "auto" | "related" | "suggestions" | "ideas";
  clickstream?: boolean;
  groupKeywords?: boolean;
}) {
  return postKeyword(
    input.projectId,
    "keywords/research",
    researchResultSchema,
    input,
  );
}

const serpResultSchema = z.object({
  rank: z.number(),
  title: z.string(),
  url: z.string(),
  domain: z.string(),
  description: z.string(),
  etv: z.number().nullable(),
  estimatedPaidTrafficCost: z.number().nullable(),
  referringDomains: z.number().nullable(),
  backlinks: z.number().nullable(),
  isNew: z.boolean(),
  rankChange: z.number().nullable(),
});

const serpAnalysisResultSchema = z.object({
  requestedKeyword: z.string(),
  items: z.array(serpResultSchema),
  depth: z.number(),
  // A shallow snapshot answering a deep request, or a refetch after provider
  // trouble: the Go API may serve a cached result at a different depth.
  reason: z.string().optional(),
});

export function getSerpAnalysis(input: {
  projectId: string;
  keyword: string;
  locationCode?: number;
  languageCode?: string;
  locationName?: string;
  depth?: 20 | 100;
}) {
  return postKeyword(
    input.projectId,
    "keywords/serp",
    serpAnalysisResultSchema,
    input,
  );
}

const savedKeywordSchema = z.object({
  id: z.string(),
  projectId: z.string(),
  keyword: z.string(),
  locationCode: z.number(),
  languageCode: z.string(),
  createdAt: z.string(),
  searchVolume: z.number().nullable(),
  cpc: z.number().nullable(),
  competition: z.number().nullable(),
  keywordDifficulty: z.number().nullable(),
  intent: z.string().nullable(),
  monthlySearches: z.array(monthlySearchSchema),
  fetchedAt: z.string().nullable(),
  tags: z.array(tagSchema),
});

const savedKeywordsResultSchema = z.object({
  rows: z.array(savedKeywordSchema),
  totalCount: z.number(),
  tags: z.array(z.object({ ...tagSchema.shape, keywordCount: z.number() })),
});

export function getSavedKeywords(input: {
  projectId: string;
  search?: string;
  includeTerms?: string[];
  excludeTerms?: string[];
  minVolume?: number | null;
  maxVolume?: number | null;
  minCpc?: number | null;
  maxCpc?: number | null;
  minDifficulty?: number | null;
  maxDifficulty?: number | null;
  tagIds?: string[];
  tagNames?: string[];
  page?: number;
  pageSize?: 50 | 100 | 250;
  sort?:
    | "createdAt"
    | "keyword"
    | "searchVolume"
    | "cpc"
    | "competition"
    | "keywordDifficulty"
    | "fetchedAt";
  order?: "asc" | "desc";
}) {
  return postKeyword(
    input.projectId,
    "keywords/saved/list",
    savedKeywordsResultSchema,
    input,
  );
}

export function exportSavedKeywords(input: {
  projectId: string;
  search?: string;
  includeTerms?: string[];
  excludeTerms?: string[];
  minVolume?: number | null;
  maxVolume?: number | null;
  minCpc?: number | null;
  maxCpc?: number | null;
  minDifficulty?: number | null;
  maxDifficulty?: number | null;
  tagIds?: string[];
  tagNames?: string[];
  sort?:
    | "createdAt"
    | "keyword"
    | "searchVolume"
    | "cpc"
    | "competition"
    | "keywordDifficulty"
    | "fetchedAt";
  order?: "asc" | "desc";
}) {
  return postKeyword(
    input.projectId,
    "keywords/saved/export",
    z.object({ rows: z.array(savedKeywordSchema) }),
    input,
  );
}

export function saveKeywords(input: {
  projectId: string;
  keywords: string[];
  locationCode?: number;
  languageCode?: string;
  tags?: string[];
  tagMode?: "append" | "replace";
  metrics?: Array<{
    keyword: string;
    searchVolume?: number | null;
    cpc?: number | null;
    competition?: number | null;
    keywordDifficulty?: number | null;
    intent?: string | null;
    monthlySearches?: MonthlySearch[];
  }>;
}) {
  return postKeyword(
    input.projectId,
    "keywords/saved/save",
    z.object({
      success: z.literal(true),
      savedKeywordIds: z.array(z.string()),
    }),
    input,
  );
}

export function removeSavedKeywords(input: {
  projectId: string;
  savedKeywordIds: string[];
}) {
  return postKeyword(
    input.projectId,
    "keywords/saved/remove",
    z.object({ success: z.literal(true), deletedCount: z.number() }),
    input,
  );
}

const assignTagsResultSchema = z.object({
  success: z.literal(true),
  taggedCount: z.number(),
  addedTags: z.array(tagSchema),
  removedTagIds: z.array(z.string()),
  removedAssignments: z.number(),
});

export function updateSavedKeywordTags(input: {
  projectId: string;
  savedKeywordIds: string[];
  addTags?: string[];
  removeTagIds?: string[];
}) {
  return postKeyword(
    input.projectId,
    "keywords/saved/tags/assign",
    assignTagsResultSchema,
    input,
  );
}

const updateTagResultSchema = z.object({
  success: z.boolean(),
  tag: tagSchema.nullable().optional(),
});

export function updateSavedKeywordTag(input: {
  projectId: string;
  tagId: string;
  name?: string;
  color?: (typeof TAG_COLOR_KEYS)[number] | null;
}) {
  return postKeyword(
    input.projectId,
    "keywords/saved/tags/update",
    updateTagResultSchema,
    input,
  );
}

export function deleteSavedKeywordTag(input: {
  projectId: string;
  tagId: string;
}) {
  return postKeyword(
    input.projectId,
    "keywords/saved/tags/delete",
    z.object({ success: z.boolean() }),
    input,
  );
}

export function refreshSavedKeywordMetrics(input: { projectId: string }) {
  return postKeyword(
    input.projectId,
    "keywords/saved/refresh",
    z.object({ updated: z.number() }),
    input,
  );
}

/* Playwright runs the keyword-research page against canned data instead of
 * the billed DataForSEO calls. */
async function loadResearchFixtures() {
  return import("../../../../tests/fixtures/keyword-research-fixtures");
}

type ResearchRequest = Parameters<typeof researchKeywords>[0];

function researchOrFixture(input: ResearchRequest) {
  if (import.meta.env.VITE_E2E_KEYWORD_FIXTURES === "1") {
    return loadResearchFixtures()
      .then((fixtures) =>
        fixtures.getKeywordResearchFixture({
          keywords: input.keywords,
          locationCode: input.locationCode ?? 2840,
          languageCode: input.languageCode ?? "en",
          projectId: input.projectId,
          resultLimit: input.resultLimit ?? 150,
          mode: input.mode ?? "auto",
          clickstream: input.clickstream ?? false,
          groupKeywords: input.groupKeywords ?? false,
        }),
      )
      .then((value) => researchResultSchema.parse(value));
  }
  return researchKeywords(input);
}

export function getKeywordResearch({ data }: { data: ResearchRequest }) {
  return researchOrFixture(data);
}
