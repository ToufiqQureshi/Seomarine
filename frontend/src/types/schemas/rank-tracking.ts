import { z } from "zod";

// ---------------------------------------------------------------------------
// DB-derived types
// ---------------------------------------------------------------------------

export type RankTrackingConfig = {
  id: string;
  projectId: string;
  domain: string;
  locationCode: number;
  languageCode: string;
  devices: "both" | "desktop" | "mobile";
  serpDepth: number;
  scheduleInterval: "daily" | "weekly" | "monthly" | "manual";
  locationName: string | null;
  isActive: boolean;
  lastCheckedAt: string | null;
  nextCheckAt: string | null;
  lastSkipReason: string | null;
  createdAt: string;
};

// ---------------------------------------------------------------------------
// API / UI types
// ---------------------------------------------------------------------------

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
