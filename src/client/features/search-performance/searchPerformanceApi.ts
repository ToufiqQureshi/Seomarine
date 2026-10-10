import { apiRequest } from "@/client/lib/seomarineApi";
import {
  searchPerformanceInputSchema,
  searchPerformanceTableExportInputSchema,
  searchPerformanceTableInputSchema,
} from "@/types/schemas/search-performance";
import { z } from "zod";

const metricsSchema = z.object({
  clicks: z.number(),
  impressions: z.number(),
  ctr: z.number(),
  position: z.number(),
});

const dimensionRowSchema = z
  .object({ key: z.string() })
  .extend(metricsSchema.shape);

const reportSchema = z.discriminatedUnion("connected", [
  z.object({ connected: z.literal(false) }),
  z.object({
    connected: z.literal(true),
    range: z.object({
      startDate: z.string(),
      endDate: z.string(),
      prevStartDate: z.string(),
      prevEndDate: z.string(),
    }),
    totals: metricsSchema,
    prevTotals: metricsSchema,
    strikingDistance: z.array(
      z.object({
        query: z.string(),
        page: z.string(),
        clicks: z.number(),
        impressions: z.number(),
        position: z.number(),
      }),
    ),
    countries: z.array(dimensionRowSchema),
  }),
]);

const tableSchema = z.discriminatedUnion("connected", [
  z.object({ connected: z.literal(false) }),
  z.object({
    connected: z.literal(true),
    dimension: z.enum(["query", "page"]),
    page: z.number(),
    pageSize: z.number(),
    hasNextPage: z.boolean(),
    totalCount: z.number().nullable(),
    rows: z.array(dimensionRowSchema),
  }),
]);

function reportPath(projectId: string, suffix: string) {
  return `/api/v1/projects/${encodeURIComponent(projectId)}/gsc/search-performance/${suffix}`;
}

export async function getSearchPerformanceReport({
  data,
}: {
  data: z.input<typeof searchPerformanceInputSchema>;
}) {
  const parsed = searchPerformanceInputSchema.parse(data);
  const { projectId, ...input } = parsed;
  return apiRequest(
    reportPath(projectId, "report"),
    reportSchema,
    "POST",
    input,
  );
}

export async function getSearchPerformanceTable({
  data,
}: {
  data: z.input<typeof searchPerformanceTableInputSchema>;
}) {
  const parsed = searchPerformanceTableInputSchema.parse(data);
  const { projectId, ...input } = parsed;
  return apiRequest(reportPath(projectId, "table"), tableSchema, "POST", input);
}

export async function exportSearchPerformanceTable({
  data,
}: {
  data: z.input<typeof searchPerformanceTableExportInputSchema>;
}) {
  const parsed = searchPerformanceTableExportInputSchema.parse(data);
  const { projectId, ...input } = parsed;
  return apiRequest(
    reportPath(projectId, "export"),
    z.object({
      dimension: z.enum(["query", "page"]),
      rows: z.array(dimensionRowSchema),
    }),
    "POST",
    input,
  );
}
