import { z } from "zod";
import { RESEARCH_SCOPES, type ResearchScope } from "@/shared/researchScope";

export const backlinksTabSchema = z.enum(["backlinks", "domains", "pages"]);

/**
 * Backlinks uses the shared research scopes. DataForSEO has no prefix
 * targeting, so subfolder rides on url_to/url prefix filters: backlink rows
 * and top pages are provider-filtered, summary counts come from filtered
 * backlink totals, and rank/trends/referring-domains stay unavailable.
 */
export type BacklinksTargetScope = ResearchScope;

/**
 * Scope as it arrives from persisted or external state (URLs, MCP clients).
 * "page" is the pre-research-scope name for exact_url and is still accepted.
 */
const backlinksScopeWithLegacySchema = z.enum([...RESEARCH_SCOPES, "page"]);

type BacklinksScopeWithLegacy = z.infer<typeof backlinksScopeWithLegacySchema>;

function resolveBacklinksScope(scope: BacklinksScopeWithLegacy): ResearchScope {
  return scope === "page" ? "exact_url" : scope;
}

const backlinksScopeParamSchema = backlinksScopeWithLegacySchema.transform(
  resolveBacklinksScope,
);

export const DEFAULT_BACKLINKS_SPAM_THRESHOLD = 40;

/* ------------------------------------------------------------------ */
/*  Paginated tab requests                                             */
/* ------------------------------------------------------------------ */

export const BACKLINKS_PAGE_SIZES = [50, 100, 200] as const;
export const DEFAULT_BACKLINKS_PAGE_SIZE = 50;

const optionalNumber = z
  .union([
    z.number(),
    z.string().transform((value, ctx) => {
      const trimmed = value.trim();
      if (trimmed === "") return undefined;
      const parsed = Number(trimmed);
      if (!Number.isFinite(parsed)) {
        ctx.addIssue({ code: "custom", message: "Invalid number" });
        return z.NEVER;
      }
      return parsed;
    }),
  ])
  .optional();

export const backlinksRowsFiltersSchema = z.object({
  include: z.string().optional(),
  exclude: z.string().optional(),
  minDomainRank: optionalNumber,
  maxDomainRank: optionalNumber,
  minLinkAuthority: optionalNumber,
  maxLinkAuthority: optionalNumber,
  minSpamScore: optionalNumber,
  maxSpamScore: optionalNumber,
  linkType: z.enum(["dofollow", "nofollow"]).optional(),
  hideLost: z.boolean().optional(),
  hideBroken: z.boolean().optional(),
  /** Exact-match on the linking domain; used to expand one domain's links. */
  domainFrom: z.string().max(255).optional(),
});

export const referringDomainsFiltersSchema = z.object({
  include: z.string().optional(),
  exclude: z.string().optional(),
  minBacklinks: optionalNumber,
  maxBacklinks: optionalNumber,
  minRank: optionalNumber,
  maxRank: optionalNumber,
  minSpamScore: optionalNumber,
  maxSpamScore: optionalNumber,
});

export const topPagesFiltersSchema = z.object({
  include: z.string().optional(),
  exclude: z.string().optional(),
  minBacklinks: optionalNumber,
  maxBacklinks: optionalNumber,
  minReferringDomains: optionalNumber,
  maxReferringDomains: optionalNumber,
  minRank: optionalNumber,
  maxRank: optionalNumber,
});

export const backlinksSortOrderSchema = z.enum(["asc", "desc"]);
// Sort field names double as table column ids on the client; the server maps
// them to DataForSEO field names.
export const backlinksRowsSortFieldSchema = z.enum([
  "rank",
  "domainRank",
  "spamScore",
  "firstSeen",
]);
export const referringDomainsSortFieldSchema = z.enum([
  "domain",
  "backlinks",
  "referringPages",
  "rank",
  "spamScore",
  "firstSeen",
  "brokenBacklinks",
]);
export const topPagesSortFieldSchema = z.enum([
  "backlinks",
  "referringDomains",
  "rank",
  "brokenBacklinks",
]);

/** Single source for each tab's default sort, shared by the request-schema
 * defaults and the client's header indicators / query fallbacks. */
export const BACKLINKS_DEFAULT_SORT = {
  backlinks: { field: "firstSeen", order: "desc" },
  domains: { field: "backlinks", order: "desc" },
  pages: { field: "backlinks", order: "desc" },
} as const satisfies Record<
  z.infer<typeof backlinksTabSchema>,
  { field: string; order: z.infer<typeof backlinksSortOrderSchema> }
>;

export const backlinksSearchSchema = z.object({
  includeSpam: z.boolean().optional().catch(undefined),
  target: z.string().optional(),
  scope: backlinksScopeParamSchema.optional().catch(undefined),
  tab: backlinksTabSchema.optional().catch(undefined),
  page: z.coerce.number().int().positive().optional().catch(undefined),
  size: z.coerce
    .number()
    .int()
    .refine((value) =>
      (BACKLINKS_PAGE_SIZES as readonly number[]).includes(value),
    )
    .optional()
    .catch(undefined),
  // Sort column id for the active tab; validated against the tab's sort-field
  // enum when building the request, so a mismatched value falls back to the
  // tab's default sort.
  sort: z.string().optional().catch(undefined),
  order: backlinksSortOrderSchema.optional().catch(undefined),
  // Backlinks tab only: "all" shows every link; default is one per domain.
  view: z.literal("all").optional().catch(undefined),
});

export type BacklinksTab = z.infer<typeof backlinksTabSchema>;
export type BacklinksSortOrder = z.infer<typeof backlinksSortOrderSchema>;
export type BacklinksRowsSortField = z.infer<
  typeof backlinksRowsSortFieldSchema
>;
export type ReferringDomainsSortField = z.infer<
  typeof referringDomainsSortFieldSchema
>;
export type TopPagesSortField = z.infer<typeof topPagesSortFieldSchema>;
export type BacklinksRowsFilters = z.infer<typeof backlinksRowsFiltersSchema>;
export type ReferringDomainsFilters = z.infer<
  typeof referringDomainsFiltersSchema
>;
export type TopPagesFilters = z.infer<typeof topPagesFiltersSchema>;
