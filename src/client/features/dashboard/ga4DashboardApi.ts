import { z } from "zod";
import { ApiError, apiRequest } from "@/client/lib/seomarineApi";

const valueSchema = z.union([z.string(), z.number(), z.null()]);
const rowSchema = z.record(z.string(), valueSchema);
const isoDateSchema = z.string().regex(/^\d{4}-\d{2}-\d{2}$/);

const organicOverviewSchema = z.object({
  request: z.object({
    resolvedDateRange: z.object({
      startDate: isoDateSchema,
      endDate: isoDateSchema,
    }),
  }),
  current: rowSchema.nullable(),
  previous: rowSchema.nullable(),
  trend: z.array(rowSchema),
});

type Totals = {
  sessions: number | null;
  activeUsers: number | null;
  engagementRate: number | null;
  keyEvents: number | null;
};

export type Ga4DashboardReport =
  | { connected: false }
  | {
      connected: true;
      totals: Totals;
      prevTotals: Totals;
      trend: Array<{ date: string; sessions: number }>;
    };

function metric(
  row: Record<string, string | number | null> | null,
  name: string,
) {
  const value = row?.[name];
  return typeof value === "number" ? value : null;
}

function totals(row: Record<string, string | number | null> | null): Totals {
  return {
    sessions: metric(row, "sessions"),
    activeUsers: metric(row, "activeUsers"),
    engagementRate: metric(row, "engagementRate"),
    keyEvents: metric(row, "keyEvents"),
  };
}

function nextDate(date: string) {
  const next = new Date(`${date}T00:00:00.000Z`);
  next.setUTCDate(next.getUTCDate() + 1);
  return next.toISOString().slice(0, 10);
}

function fillDailySessions(
  rows: Array<Record<string, string | number | null>>,
  range: { startDate: string; endDate: string },
) {
  const sessionsByDate = new Map<string, number>();
  for (const row of rows) {
    if (typeof row.date !== "string" || typeof row.sessions !== "number") {
      continue;
    }
    const match = /^(\d{4})(\d{2})(\d{2})$/.exec(row.date);
    if (!match) continue;
    sessionsByDate.set(`${match[1]}-${match[2]}-${match[3]}`, row.sessions);
  }

  const days: Array<{ date: string; sessions: number }> = [];
  for (
    let date = range.startDate;
    date <= range.endDate;
    date = nextDate(date)
  ) {
    days.push({ date, sessions: sessionsByDate.get(date) ?? 0 });
  }
  return days;
}

function projectPath(projectId: string) {
  return `/api/v1/projects/${encodeURIComponent(projectId)}/ga4/overview/organic`;
}

export async function getGa4DashboardReport(
  projectId: string,
): Promise<Ga4DashboardReport> {
  try {
    const overview = await apiRequest(
      projectPath(projectId),
      organicOverviewSchema,
      "POST",
      {},
    );
    return {
      connected: true,
      totals: totals(overview.current),
      prevTotals: totals(overview.previous),
      trend: fillDailySessions(
        overview.trend,
        overview.request.resolvedDateRange,
      ),
    };
  } catch (error) {
    if (
      error instanceof ApiError &&
      [
        "ga4_not_connected",
        "ga4_reconnect_required",
        "ga4_property_inaccessible",
      ].includes(error.code)
    ) {
      return { connected: false };
    }
    throw error;
  }
}
