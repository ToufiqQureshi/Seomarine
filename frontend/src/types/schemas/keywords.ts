import { z } from "zod";
import {
  booleanSearchParamSchema,
  optionalSearchNumberParam,
  optionalSearchPositiveIntParam,
  searchTextParam,
} from "@/types/schemas/domain";

const savedKeywordTagSchema = z.string().trim().min(1).max(64);
const savedKeywordSortFields = [
  "createdAt",
  "keyword",
  "searchVolume",
  "cpc",
  "competition",
  "keywordDifficulty",
  "fetchedAt",
] as const;
const sortDirs = ["asc", "desc"] as const;
const savedKeywordPageSizeSchema = z.union([
  z.literal(50),
  z.literal(100),
  z.literal(250),
]);

const researchKeywordsSchema = z.object({
  projectId: z.string().min(1),
  keywords: z.array(z.string().min(1)).min(1).max(200),
  locationCode: z.number().int().positive().optional(),
  languageCode: z.string().min(2).max(8).optional(),
  // Canonical DataForSEO name of a city, county, or region inside the
  // country. Scopes volume, CPC, and competition to that area.
  locationName: z.string().trim().min(1).max(200).optional(),
  resultLimit: z
    .union([z.literal(150), z.literal(300), z.literal(500)])
    .default(150),
  mode: z
    .enum(["auto", "related", "suggestions", "ideas"])
    .optional()
    .default("auto"),
  // Clickstream-refined volumes double the DataForSEO request cost; opt-in.
  clickstream: z.boolean().optional().default(false),
  groupKeywords: z.boolean().optional().default(false),
});

const savedKeywordMetricSchema = z.object({
  keyword: z.string().min(1),
  searchVolume: z.number().int().nonnegative().nullable().optional(),
  cpc: z.number().nonnegative().nullable().optional(),
  competition: z.number().min(0).max(1).nullable().optional(),
  keywordDifficulty: z.number().int().min(0).max(100).nullable().optional(),
  intent: z
    .enum([
      "informational",
      "commercial",
      "transactional",
      "navigational",
      "unknown",
    ])
    .nullable()
    .optional(),
  monthlySearches: z
    .array(
      z.object({
        year: z.number().int().positive(),
        month: z.number().int().min(1).max(12),
        searchVolume: z.number().int().nonnegative(),
      }),
    )
    .optional(),
});

export const saveKeywordsSchema = z
  .object({
    projectId: z.string().min(1),
    keywords: z.array(z.string().min(1)).min(1).max(500),
    locationCode: z.number().int().positive().optional(),
    languageCode: z.string().min(2).max(8).optional(),
    tags: z.array(savedKeywordTagSchema).max(20).optional(),
    tagMode: z.enum(["append", "replace"]).optional(),
    metrics: z.array(savedKeywordMetricSchema).max(500).optional(),
  })
  .refine(
    (value) => value.tagMode !== "replace" || (value.tags?.length ?? 0) > 0,
    "Replacement tags are required when tagMode is replace.",
  );

export const getSavedKeywordsSchema = z.object({
  projectId: z.string().min(1),
  search: z.string().trim().max(200).optional(),
  includeTerms: z.array(z.string().trim().min(1)).max(20).optional(),
  excludeTerms: z.array(z.string().trim().min(1)).max(20).optional(),
  minVolume: z.number().int().nonnegative().nullable().optional(),
  maxVolume: z.number().int().nonnegative().nullable().optional(),
  minCpc: z.number().nonnegative().nullable().optional(),
  maxCpc: z.number().nonnegative().nullable().optional(),
  minDifficulty: z.number().int().min(0).max(100).nullable().optional(),
  maxDifficulty: z.number().int().min(0).max(100).nullable().optional(),
  tagIds: z.array(z.string().min(1)).max(50).optional(),
  tagNames: z.array(savedKeywordTagSchema).max(50).optional(),
  page: z.number().int().positive().default(1),
  pageSize: savedKeywordPageSizeSchema.default(50),
  sort: z.enum(savedKeywordSortFields).default("createdAt"),
  order: z.enum(sortDirs).default("desc"),
});

/**
 * /p/$projectId/saved query params. `sort=createdAt` is the unsorted table:
 * the server's insertion order.
 */
export const savedKeywordsSearchSchema = z.object({
  include: searchTextParam,
  exclude: searchTextParam,
  minVol: optionalSearchNumberParam,
  maxVol: optionalSearchNumberParam,
  minCpc: optionalSearchNumberParam,
  maxCpc: optionalSearchNumberParam,
  minKd: optionalSearchNumberParam,
  maxKd: optionalSearchNumberParam,
  tags: z.array(z.string()).optional().catch(undefined),
  sort: z.enum(savedKeywordSortFields).optional().catch(undefined),
  order: z.enum(sortDirs).optional().catch(undefined),
  page: optionalSearchPositiveIntParam,
  size: z.coerce
    .number()
    .pipe(savedKeywordPageSizeSchema)
    .optional()
    .catch(undefined),
});

export type SavedKeywordsSearch = z.infer<typeof savedKeywordsSearchSchema>;

export const exportSavedKeywordsSchema = getSavedKeywordsSchema.omit({
  page: true,
  pageSize: true,
});

type ResearchKeywordsInput = z.infer<typeof researchKeywordsSchema>;
export type SaveKeywordsInput = z.infer<typeof saveKeywordsSchema>;
type ResolvedMarket = { locationCode: number; languageCode: string };
export type ResolvedResearchKeywordsInput = Omit<
  ResearchKeywordsInput,
  keyof ResolvedMarket
> &
  ResolvedMarket;
export type GetSavedKeywordsInput = z.infer<typeof getSavedKeywordsSchema>;
export type ExportSavedKeywordsInput = z.infer<
  typeof exportSavedKeywordsSchema
>;

/* ------------------------------------------------------------------ */
/*  URL search params schema for /p/$projectId/keywords                */
/* ------------------------------------------------------------------ */

const keywordSortFields = [
  "keyword",
  "searchVolume",
  "cpc",
  "competition",
  "keywordDifficulty",
] as const;

const keywordModes = ["auto", "related", "suggestions", "ideas"] as const;

export const keywordsSearchSchema = z.object({
  q: z.string().optional(),
  loc: z.coerce.number().int().positive().optional(),
  locName: z.string().optional(),
  kLimit: z.union([z.literal(150), z.literal(300), z.literal(500)]).optional(),
  mode: z.enum(keywordModes).optional(),
  cs: booleanSearchParamSchema.optional(),
  grp: booleanSearchParamSchema.optional(),
  sort: z.enum(keywordSortFields).optional(),
  order: z.enum(sortDirs).optional(),
  minVol: z.string().optional(),
  maxVol: z.string().optional(),
  minCpc: z.string().optional(),
  maxCpc: z.string().optional(),
  minKd: z.string().optional(),
  maxKd: z.string().optional(),
  include: z.string().optional(),
  exclude: z.string().optional(),
});
