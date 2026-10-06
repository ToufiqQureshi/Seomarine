import type { InferSelectModel } from "drizzle-orm";
import { z } from "zod";
import { rankTrackingConfigs } from "@/db/schema";
import { isSupportedLanguageCode } from "@/shared/keyword-locations";
import { MAX_TRACKED_KEYWORD_LENGTH } from "@/shared/rank-tracking";
import { comparePeriodSchema } from "@/types/schemas/rank-tracking-search";
import { domainField } from "@/types/schemas/domain";

// ---------------------------------------------------------------------------
// DB-derived types
// ---------------------------------------------------------------------------

export type RankTrackingConfig = InferSelectModel<typeof rankTrackingConfigs>;

// ---------------------------------------------------------------------------
// API / UI types
// ---------------------------------------------------------------------------

export type RankCheckTriggerResult =
  | {
      ok: true;
      runId: string;
    }
  | {
      ok: false;
      reason: "already_running";
      blockingRunId: string | null;
    };

export interface RankTrackingDeviceResult {
  position: number | null;
  previousPosition: number | null;
  rankingUrl: string | null;
  serpFeatures: string[];
}

export interface RankTrackingRow {
  trackingKeywordId: string;
  keyword: string;
  matchCase: boolean;
  searchVolume: number | null;
  keywordDifficulty: number | null;
  cpc: number | null;
  desktop: RankTrackingDeviceResult;
  mobile: RankTrackingDeviceResult;
}

// ---------------------------------------------------------------------------
// Validation schemas
// ---------------------------------------------------------------------------

const devicesEnum = z.enum(rankTrackingConfigs.devices.enumValues);
const scheduleEnum = z.enum(rankTrackingConfigs.scheduleInterval.enumValues);
// Rank tracking runs against the SERP API, which serves any language in any
// country — but an unknown code is a *charged* DataForSEO failure, so reject
// it here at cost 0.
const languageCodeField = z
  .string()
  .max(10)
  .refine(isSupportedLanguageCode, "Unsupported language code");

function isTimeZone(timeZone: string): boolean {
  try {
    Intl.DateTimeFormat("en-US", { timeZone });
    return true;
  } catch {
    return false;
  }
}

// A user-chosen run time for scheduled checks, in their own timezone. It only
// picks where the rank_tracking_configs.next_check_at anchor starts; the
// anchor is UTC and advances in fixed steps, so the timezone is not stored.
export const scheduleTimeSchema = z.object({
  weekday: z
    .number()
    .int()
    .min(0)
    .max(6)
    .optional()
    .describe(
      "Day of week, 0 = Sunday. Required for weekly schedules, ignored by the others.",
    ),
  hour: z.number().int().min(0).max(23).describe("Hour, 0-23."),
  minute: z.number().int().min(0).max(59).describe("Minute, 0-59."),
  timeZone: z
    .string()
    .refine(isTimeZone, "Unknown IANA timezone")
    .optional()
    .describe(
      'IANA timezone the weekday, hour, and minute are in, e.g. "America/New_York". Defaults to UTC.',
    ),
});
export type RankCheckScheduleTime = z.infer<typeof scheduleTimeSchema>;

export const getConfigsSchema = z.object({
  projectId: z.string().uuid(),
});

export const createConfigSchema = z.object({
  projectId: z.string().uuid(),
  domain: domainField,
  locationCode: z.number().int().positive().optional(),
  languageCode: languageCodeField.optional(),
  locationName: z.string().min(1).max(200).optional(),
  devices: devicesEnum.optional(),
  serpDepth: z.number().int().min(10).max(100).multipleOf(10),
  scheduleInterval: scheduleEnum.optional(),
  scheduleTime: scheduleTimeSchema.optional(),
});

export const updateConfigSchema = z.object({
  projectId: z.string().uuid(),
  configId: z.string().uuid(),
  domain: domainField.optional(),
  locationCode: z.number().int().positive().optional(),
  languageCode: languageCodeField.optional(),
  locationName: z.string().min(1).max(200).nullable().optional(),
  devices: devicesEnum.optional(),
  serpDepth: z.number().int().min(10).max(100).multipleOf(10).optional(),
  scheduleInterval: scheduleEnum.optional(),
  scheduleTime: scheduleTimeSchema.optional(),
  isActive: z.boolean().optional(),
});

export const triggerCheckSchema = z.object({
  projectId: z.string().uuid(),
  configId: z.string().uuid(),
  keywordIds: z.array(z.string().uuid()).max(2000).optional(),
});

export const getLatestResultsSchema = z.object({
  projectId: z.string().uuid(),
  configId: z.string().uuid(),
  comparePeriod: comparePeriodSchema.optional(),
});

export const getLatestRunSchema = z.object({
  projectId: z.string().uuid(),
  configId: z.string().uuid(),
});

export const estimateCostSchema = z.object({
  projectId: z.string().uuid(),
  configId: z.string().uuid(),
});

export const addKeywordsSchema = z.object({
  projectId: z.string().uuid(),
  configId: z.string().uuid(),
  keywords: z
    .array(z.string().min(1).max(MAX_TRACKED_KEYWORD_LENGTH))
    .min(1)
    .max(2000),
  matchCase: z.boolean().optional(),
});

export const removeKeywordsSchema = z.object({
  projectId: z.string().uuid(),
  configId: z.string().uuid(),
  keywordIds: z.array(z.string().uuid()).min(1).max(2000),
});

export const refreshMetricsSchema = z.object({
  projectId: z.string().uuid(),
  configId: z.string().uuid(),
});

const deviceEnum = z.enum(["desktop", "mobile"]);
const sinceDaysField = z.number().int().positive().max(730).default(365);

export const getKeywordHistorySchema = z.object({
  projectId: z.string().uuid(),
  configId: z.string().uuid(),
  trackingKeywordId: z.string().uuid(),
  sinceDays: sinceDaysField,
});

export const getConfigTrendSchema = z.object({
  projectId: z.string().uuid(),
  configId: z.string().uuid(),
  device: deviceEnum,
  sinceDays: sinceDaysField,
});

export const getPositionMatrixSchema = z.object({
  projectId: z.string().uuid(),
  configId: z.string().uuid(),
  device: deviceEnum,
  runLimit: z.number().int().positive().max(26).default(12),
});
