import { useQuery } from "@tanstack/react-query";
import { z } from "zod";
import { apiRequest, shouldRetryApiError } from "@/client/lib/uplumoApi";

const featureSchema = z.enum([
  "rank_tracking",
  "keyword_research",
  "local_seo",
  "domain_research",
  "backlinks",
  "ai_visibility",
  "site_audit",
  "ai_assistant",
  "ai_writing",
]);

export type CreditFeature = z.infer<typeof featureSchema>;

const usageSchema = z.object({
  plan: z.enum(["free", "basic", "pro", "agency"]),
  unlimited: z.boolean(),
  chargesCredits: z.boolean(),
  balanceCredits: z.number().int().nonnegative(),
  monthlyCredits: z.number().int().nonnegative(),
  periodStart: z.string(),
  periodEnd: z.string().nullable(),
  spent: z.array(
    z.object({
      feature: z.union([featureSchema, z.literal("other")]),
      credits: z.number().int().positive(),
    }),
  ),
  costs: z.array(
    z.object({
      feature: featureSchema,
      unit: z.string(),
      credits: z.number().int().positive(),
    }),
  ),
});

export type BillingUsage = z.infer<typeof usageSchema>;

const FEATURE_LABELS: Record<CreditFeature | "other", string> = {
  rank_tracking: "Rank tracking",
  keyword_research: "Keyword research",
  local_seo: "Local SEO",
  domain_research: "Domain research",
  backlinks: "Backlinks",
  ai_visibility: "AI Visibility",
  site_audit: "Site audit page speed",
  ai_assistant: "AI assistant",
  ai_writing: "AI writing",
  other: "Other",
};

export function creditFeatureLabel(feature: CreditFeature | "other") {
  return FEATURE_LABELS[feature];
}

/** Below this share of the monthly credits the app warns that they run low. */
const LOW_SHARE = 0.1;

type CreditLevel = "ok" | "low" | "out" | "not_charged";

/**
 * How worried the workspace should be about its credits. Staff workspaces
 * and servers without payments are never charged.
 */
export function creditLevel(usage: BillingUsage): CreditLevel {
  if (usage.unlimited || !usage.chargesCredits) return "not_charged";
  if (usage.balanceCredits <= 0) return "out";
  if (usage.balanceCredits < usage.monthlyCredits * LOW_SHARE) return "low";
  return "ok";
}

const billingUsageQueryKey = ["billing", "usage"] as const;

/** The active workspace's credits, spend this month and typical costs. */
export function useBillingUsage({
  enabled = true,
}: { enabled?: boolean } = {}) {
  return useQuery({
    queryKey: billingUsageQueryKey,
    queryFn: () => apiRequest("/api/v1/billing/usage", usageSchema),
    enabled,
    staleTime: 30_000,
    retry: shouldRetryApiError,
  });
}
