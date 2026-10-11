import { ApiError, apiRequest } from "@/client/lib/seomarineApi";
import type {
  RankCheckScheduleTime,
  RankTrackingConfig,
} from "@/types/schemas/rank-tracking";
import { z } from "zod";

type ProjectData<T extends object = object> = {
  data: T & { projectId: string };
};

const projectIdSchema = z.string().uuid();
const configIdSchema = z.string().uuid();
const configSchema = z.object({
  id: configIdSchema,
  projectId: projectIdSchema,
  domain: z.string(),
  locationCode: z.number(),
  languageCode: z.string(),
  locationName: z.string().nullable(),
  devices: z.enum(["both", "desktop", "mobile"]),
  serpDepth: z.number(),
  scheduleInterval: z.enum(["manual", "daily", "weekly", "monthly"]),
  isActive: z.boolean(),
  lastCheckedAt: z.string().nullable(),
  nextCheckAt: z.string().nullable(),
  lastSkipReason: z.string().nullable(),
  createdAt: z.string(),
});
const configSummarySchema = configSchema.extend({
  keywordCount: z.number(),
  lastRunStatus: z.string().nullable(),
  lastRunCompletedAt: z.string().nullable(),
});
const deviceResultSchema = z.object({
  position: z.number().nullable(),
  previousPosition: z.number().nullable(),
  rankingUrl: z.string().nullable(),
  serpFeatures: z.array(z.string()),
});
const latestResultsSchema = z.object({
  rows: z.array(
    z.object({
      trackingKeywordId: z.string(),
      keyword: z.string(),
      matchCase: z.boolean(),
      searchVolume: z.number().nullable(),
      keywordDifficulty: z.number().nullable(),
      cpc: z.number().nullable(),
      desktop: deviceResultSchema,
      mobile: deviceResultSchema,
    }),
  ),
  run: z
    .object({
      id: z.string(),
      status: z.string(),
      lastCheckedAt: z.string().nullable(),
      completedAt: z.string().nullable(),
      errorMessage: z.string().nullable(),
    })
    .nullable(),
});
const projectPath = (projectId: string, suffix: string) =>
  `/api/v1/projects/${encodeURIComponent(projectId)}/rank-tracking/${suffix}`;

export type RankTrackingConfigSummary = z.infer<typeof configSummarySchema>;
export type RankKeywordHistoryPoint = {
  device: "desktop" | "mobile";
  checkedAt: string;
  position: number | null;
};
export type RankPositionMatrixCell = {
  runId: string;
  checkedAt: string;
  trackingKeywordId: string;
  position: number | null;
};

export async function getRankTrackingConfigs({ data }: ProjectData) {
  projectIdSchema.parse(data.projectId);
  const out = await apiRequest(
    projectPath(data.projectId, "configs/list"),
    z.object({ configs: z.array(configSchema) }),
    "POST",
    {},
  );
  return out.configs;
}

export async function getRankTrackingConfigSummaries({ data }: ProjectData) {
  projectIdSchema.parse(data.projectId);
  const out = await apiRequest(
    projectPath(data.projectId, "configs/summaries"),
    z.object({ summaries: z.array(configSummarySchema) }),
    "POST",
    {},
  );
  return out.summaries;
}

export async function createRankTrackingConfig({
  data,
}: ProjectData<{
  domain: string;
  locationCode?: number;
  languageCode?: string;
  locationName?: string;
  devices?: RankTrackingConfig["devices"];
  serpDepth: number;
  scheduleInterval?: RankTrackingConfig["scheduleInterval"];
  scheduleTime?: RankCheckScheduleTime;
}>) {
  const { projectId, ...input } = data;
  const out = await apiRequest(
    projectPath(projectId, "configs/create"),
    z.object({ config: configSchema }),
    "POST",
    input,
  );
  return out.config;
}

export async function updateRankTrackingConfig({
  data,
}: ProjectData<{
  configId: string;
  domain?: string;
  locationCode?: number;
  languageCode?: string;
  locationName?: string | null;
  devices?: RankTrackingConfig["devices"];
  serpDepth?: number;
  scheduleInterval?: RankTrackingConfig["scheduleInterval"];
  scheduleTime?: RankCheckScheduleTime;
  isActive?: boolean;
}>) {
  const { projectId, ...input } = data;
  configIdSchema.parse(input.configId);
  await apiRequest(
    projectPath(projectId, "configs/update"),
    z.object({ config: configSchema }),
    "POST",
    input,
  );
  return { success: true as const };
}

export async function triggerRankCheck({
  data,
}: ProjectData<{
  configId: string;
  keywordIds?: string[];
}>) {
  const { projectId, ...input } = data;
  try {
    const result = await apiRequest(
      projectPath(projectId, "checks/start"),
      z.object({ ok: z.literal(true), runId: z.string() }),
      "POST",
      input,
    );
    return result;
  } catch (error) {
    if (error instanceof ApiError && error.code === "already_running") {
      return {
        ok: false as const,
        reason: "already_running" as const,
        blockingRunId: null,
      };
    }
    throw error;
  }
}

export async function getLatestRankResults({
  data,
}: ProjectData<{
  configId: string;
  comparePeriod?: "1d" | "7d" | "30d" | "90d";
}>) {
  const { projectId, ...input } = data;
  return apiRequest(
    projectPath(projectId, "results/latest"),
    latestResultsSchema,
    "POST",
    input,
  );
}

export async function getLatestRankRun({
  data,
}: ProjectData<{ configId: string }>) {
  const { projectId, ...input } = data;
  const out = await apiRequest(
    projectPath(projectId, "runs/latest"),
    z.object({
      run: z
        .object({
          id: z.string(),
          configId: z.string(),
          projectId: z.string(),
          status: z.string(),
          keywordsTotal: z.number(),
          keywordsChecked: z.number(),
          isSubsetRun: z.boolean(),
          errorMessage: z.string().nullable(),
          startedAt: z.string(),
          completedAt: z.string().nullable(),
        })
        .nullable(),
    }),
    "POST",
    input,
  );
  return out.run ? { ...out.run, maybeStale: false } : null;
}

export async function estimateRankCheckCost({
  data,
}: ProjectData<{ configId: string }>) {
  const { projectId, ...input } = data;
  return apiRequest(
    projectPath(projectId, "estimate-cost"),
    z.object({
      keywordCount: z.number(),
      costUsd: z.number(),
      costCredits: z.number(),
    }),
    "POST",
    input,
  );
}

export async function addTrackingKeywords({
  data,
}: ProjectData<{
  configId: string;
  keywords: string[];
  matchCase?: boolean;
}>) {
  const { projectId, ...input } = data;
  const result = await apiRequest(
    projectPath(projectId, "keywords/add"),
    z.object({
      added: z.number(),
      addedIds: z.array(z.string()),
      tooLong: z.array(z.string()),
    }),
    "POST",
    input,
  );
  return { ...result, checkTriggered: false, checkScheduledSoon: false };
}

export async function removeTrackingKeywords({
  data,
}: ProjectData<{
  configId: string;
  keywordIds: string[];
}>) {
  const { projectId, ...input } = data;
  return apiRequest(
    projectPath(projectId, "keywords/remove"),
    z.object({ removed: z.number(), removedIds: z.array(z.string()) }),
    "POST",
    input,
  );
}

export async function refreshTrackingKeywordMetrics({
  data,
}: ProjectData<{ configId: string }>) {
  const { projectId, ...input } = data;
  return apiRequest(
    projectPath(projectId, "keywords/metrics/refresh"),
    z.object({ updated: z.number() }),
    "POST",
    input,
  );
}

export async function getRankKeywordHistory({
  data,
}: ProjectData<{
  configId: string;
  trackingKeywordId: string;
  sinceDays: number;
}>) {
  const { projectId, ...input } = data;
  const out = await apiRequest(
    projectPath(projectId, "results/history"),
    z.object({
      points: z.array(
        z.object({
          device: z.enum(["desktop", "mobile"]),
          checkedAt: z.string(),
          position: z.number().nullable(),
        }),
      ),
    }),
    "POST",
    input,
  );
  return out.points;
}

export async function getRankConfigTrend({
  data,
}: ProjectData<{
  configId: string;
  device: "desktop" | "mobile";
  sinceDays: number;
}>) {
  const { projectId, ...input } = data;
  const out = await apiRequest(
    projectPath(projectId, "results/trend"),
    z.object({
      points: z.array(
        z.object({
          runId: z.string(),
          checkedAt: z.string(),
          total: z.number(),
          top3: z.number(),
          top4to10: z.number(),
          top11to20: z.number(),
        }),
      ),
    }),
    "POST",
    input,
  );
  return out.points.map((point) => ({
    ...point,
    notRanking: Math.max(
      0,
      point.total - point.top3 - point.top4to10 - point.top11to20,
    ),
  }));
}

export async function getRankPositionMatrix({
  data,
}: ProjectData<{
  configId: string;
  device: "desktop" | "mobile";
  runLimit?: number;
}>) {
  const { projectId, ...input } = data;
  const out = await apiRequest(
    projectPath(projectId, "results/matrix"),
    z.object({
      points: z.array(
        z.object({
          runId: z.string(),
          checkedAt: z.string(),
          trackingKeywordId: z.string(),
          position: z.number().nullable(),
        }),
      ),
    }),
    "POST",
    input,
  );
  return out.points;
}
