import { z } from "zod";
import { apiRequest, shouldRetryApiError } from "@/client/lib/seomarineApi";

export const RANGE_DAYS = [7, 30, 90] as const;
export type RangeDays = (typeof RANGE_DAYS)[number];

const count = z.number().int().nonnegative();
const isoDate = z.iso.date();

export const CHANNELS = [
  "ai",
  "search",
  "social",
  "direct",
  "referral",
] as const;
export type Channel = (typeof CHANNELS)[number];

export const AI_SOURCES = [
  "chatgpt",
  "perplexity",
  "gemini",
  "claude",
  "copilot",
  "other_ai",
] as const;
export type AiSource = (typeof AI_SOURCES)[number];

const summarySchema = z.object({
  visitors: count,
  pageviews: count,
  series: z.array(
    z.object({ date: isoDate, visitors: count, pageviews: count }),
  ),
  topPages: z.array(z.object({ path: z.string(), pageviews: count })),
  channels: z.array(z.object({ channel: z.enum(CHANNELS), visitors: count })),
  aiSources: z.array(z.object({ source: z.enum(AI_SOURCES), visitors: count })),
  referrers: z.array(z.object({ host: z.string(), visitors: count })),
  devices: z.array(
    z.object({
      device: z.enum(["desktop", "mobile", "tablet"]),
      visitors: count,
    }),
  ),
});
export type AnalyticsSummary = z.infer<typeof summarySchema>;

const siteSchema = z.object({
  id: z.number().int().positive(),
  siteKey: z.string().min(1),
  snippet: z.string().min(1),
});
type AnalyticsSite = z.infer<typeof siteSchema>;

/**
 * The last `days` whole days, today included. The Go API counts in UTC days,
 * so the range is in UTC too.
 */
export function rangeForDays(days: RangeDays, now: Date) {
  const to = now.toISOString().slice(0, 10);
  const start = new Date(now);
  start.setUTCDate(start.getUTCDate() - (days - 1));
  return { from: start.toISOString().slice(0, 10), to };
}

function projectPath(projectId: string) {
  return `/api/v1/projects/${encodeURIComponent(projectId)}/analytics`;
}

export function analyticsSummaryQueryOptions(
  projectId: string,
  range: { from: string; to: string },
) {
  return {
    queryKey: ["goAnalyticsSummary", projectId, range.from, range.to],
    queryFn: () =>
      apiRequest(
        `${projectPath(projectId)}/summary?${new URLSearchParams(range)}`,
        summarySchema,
      ),
    retry: shouldRetryApiError,
    // Visits arrive all the time: refetch on focus after a minute.
    staleTime: 60_000,
  };
}

export function createAnalyticsSite(projectId: string): Promise<AnalyticsSite> {
  return apiRequest(`${projectPath(projectId)}/site`, siteSchema, "POST");
}

const countrySchema = z.object({
  code: z.string().regex(/^[A-Z]{2}$/),
  name: z.string().min(1),
  visitors: count,
  pct: z.number().min(0).max(100),
});
type Country = z.infer<typeof countrySchema>;

export function analyticsSiteQueryOptions(projectId: string) {
  return {
    queryKey: ["goAnalyticsSite", projectId],
    queryFn: () => createAnalyticsSite(projectId),
    retry: shouldRetryApiError,
    staleTime: Infinity,
  };
}

export function analyticsCountriesPage(
  siteId: number,
  range: { from: string; to: string },
  page: number,
): Promise<Country[]> {
  const query = new URLSearchParams({
    ...range,
    page: String(page),
    limit: "10",
  });
  return apiRequest(
    `/api/v1/analytics/${siteId}/countries?${query}`,
    z.array(countrySchema),
  );
}
