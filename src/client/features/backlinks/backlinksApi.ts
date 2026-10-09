import { z } from "zod";
import { apiRequest } from "@/client/lib/seomarineApi";
import type {
  BacklinksRowsFilters,
  BacklinksRowsSortField,
  BacklinksSortOrder,
  BacklinksTargetScope,
  ReferringDomainsFilters,
  ReferringDomainsSortField,
  TopPagesFilters,
  TopPagesSortField,
} from "@/types/schemas/backlinks";

const nullableNumber = z.number().nullable();
const backlinkRowSchema = z.object({
  domainFrom: z.string().nullable(),
  urlFrom: z.string().nullable(),
  urlTo: z.string().nullable(),
  anchor: z.string().nullable(),
  itemType: z.string().nullable(),
  isDofollow: z.boolean().nullable(),
  relAttributes: z.array(z.string()),
  rank: nullableNumber,
  domainFromRank: nullableNumber,
  pageFromRank: nullableNumber,
  spamScore: nullableNumber,
  firstSeen: z.string().nullable(),
  lastSeen: z.string().nullable(),
  isLost: z.boolean(),
  isBroken: z.boolean(),
  linksCount: nullableNumber,
});
const referringDomainSchema = z.object({
  domain: z.string().nullable(),
  backlinks: nullableNumber,
  referringPages: nullableNumber,
  rank: nullableNumber,
  spamScore: nullableNumber,
  firstSeen: z.string().nullable(),
  brokenBacklinks: nullableNumber,
  brokenPages: nullableNumber,
});
const topPageSchema = z.object({
  page: z.string().nullable(),
  backlinks: nullableNumber,
  referringDomains: nullableNumber,
  rank: nullableNumber,
  brokenBacklinks: nullableNumber,
});
const pageSchema = <T extends z.ZodTypeAny>(row: T) =>
  z.object({
    rows: z.array(row),
    totalCount: z.number().nullable(),
    hasMore: z.boolean(),
    page: z.number(),
    pageSize: z.number(),
    fetchedAt: z.string(),
  });
export const backlinksOverviewSchema = z.object({
  target: z.string(),
  displayTarget: z.string(),
  scope: z.enum(["exact_url", "subfolder", "domain", "subdomains"]),
  summary: z.object({
    rank: nullableNumber,
    backlinks: nullableNumber,
    referringPages: nullableNumber,
    referringDomains: nullableNumber,
    brokenBacklinks: nullableNumber,
    brokenPages: nullableNumber,
    backlinksSpamScore: nullableNumber,
    targetSpamScore: nullableNumber,
    newBacklinks: nullableNumber,
    lostBacklinks: nullableNumber,
    newReferringDomains: nullableNumber,
    lostReferringDomains: nullableNumber,
  }),
  trends: z.array(
    z.object({
      date: z.string(),
      backlinks: nullableNumber,
      referringDomains: nullableNumber,
      rank: nullableNumber,
    }),
  ),
  newLostTrends: z.array(
    z.object({
      date: z.string(),
      newBacklinks: nullableNumber,
      lostBacklinks: nullableNumber,
      newReferringDomains: nullableNumber,
      lostReferringDomains: nullableNumber,
    }),
  ),
  fetchedAt: z.string(),
});
const backlinksRowsPageSchema = pageSchema(backlinkRowSchema);
const referringDomainsPageSchema = pageSchema(referringDomainSchema);
const topPagesPageSchema = pageSchema(topPageSchema);
export type BacklinksOverviewData = z.infer<typeof backlinksOverviewSchema>;
export type BacklinksRowsPageData = z.infer<typeof backlinksRowsPageSchema>;
export type ReferringDomainsPageData = z.infer<
  typeof referringDomainsPageSchema
>;
export type TopPagesPageData = z.infer<typeof topPagesPageSchema>;
export type BacklinksRow = BacklinksRowsPageData["rows"][number];
export type ReferringDomainRow = ReferringDomainsPageData["rows"][number];
export type TopPageRow = TopPagesPageData["rows"][number];

type Lookup = {
  projectId: string;
  target: string;
  scope?: BacklinksTargetScope;
};
type PageInput = Lookup & {
  page: number;
  pageSize: number;
  sortOrder: BacklinksSortOrder;
};
function path(projectId: string, endpoint: string) {
  return `/api/v1/projects/${encodeURIComponent(projectId)}/backlinks/${endpoint}`;
}
export function getBacklinksOverview(
  input: Lookup,
): Promise<BacklinksOverviewData> {
  const { projectId, ...body } = input;
  return apiRequest(
    path(projectId, "overview"),
    backlinksOverviewSchema,
    "POST",
    body,
  );
}
export function getBacklinksRows(
  input: PageInput & {
    hideSpam?: boolean;
    sortField: BacklinksRowsSortField;
    filters: BacklinksRowsFilters;
    mode: "one_per_domain" | "as_is";
  },
): Promise<BacklinksRowsPageData> {
  const { projectId, ...body } = input;
  return apiRequest(
    path(projectId, "rows"),
    backlinksRowsPageSchema,
    "POST",
    body,
  );
}
export function getBacklinksReferringDomains(
  input: PageInput & {
    sortField: ReferringDomainsSortField;
    filters: ReferringDomainsFilters;
  },
): Promise<ReferringDomainsPageData> {
  const { projectId, ...body } = input;
  return apiRequest(
    path(projectId, "referring-domains"),
    referringDomainsPageSchema,
    "POST",
    body,
  );
}
export function getBacklinksTopPages(
  input: PageInput & { sortField: TopPagesSortField; filters: TopPagesFilters },
): Promise<TopPagesPageData> {
  const { projectId, ...body } = input;
  return apiRequest(
    path(projectId, "top-pages"),
    topPagesPageSchema,
    "POST",
    body,
  );
}
