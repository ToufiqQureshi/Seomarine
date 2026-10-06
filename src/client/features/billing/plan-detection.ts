import {
  AUTUMN_PAID_PLAN_FEATURE_ID,
  AUTUMN_SEO_DATA_BALANCE_FEATURE_ID,
  autumnSeoDataCreditsToUsd,
} from "@/shared/billing";

export type PlanStatus = "free" | "paid";

// The slice of the Autumn customer we read. Structural so tests and both
// the SDK's React and server shapes fit without casts.
type CustomerLike = {
  flags?: Record<string, { planId?: string | null } | undefined>;
  subscriptions?: Array<{ planId: string; plan?: { name: string } }>;
  balances?: Record<string, { granted: number } | undefined>;
};

export function getCustomerPlanStatus(
  customer: CustomerLike | undefined,
): PlanStatus {
  return customer?.flags?.[AUTUMN_PAID_PLAN_FEATURE_ID] ? "paid" : "free";
}

// Autumn records which plan granted the paid entitlement, so any plan that
// grants paid_plan is recognized without listing plan IDs here.
export function getCustomerPaidPlanId(
  customer: CustomerLike | undefined,
): string | null {
  return customer?.flags?.[AUTUMN_PAID_PLAN_FEATURE_ID]?.planId ?? null;
}

// Display details for the plan a paid customer is on. `name` needs the
// customer fetched with `expand: ["subscriptions.plan"]`; it falls back to
// the plan ID otherwise.
export function getCustomerPaidPlan(customer: CustomerLike | undefined) {
  const planId = getCustomerPaidPlanId(customer);
  if (!planId) return null;
  const subscription = customer?.subscriptions?.find(
    (s) => s.planId === planId,
  );
  return {
    id: planId,
    name: subscription?.plan?.name ?? planId,
    monthlyCreditsUsd: autumnSeoDataCreditsToUsd(
      customer?.balances?.[AUTUMN_SEO_DATA_BALANCE_FEATURE_ID]?.granted ?? 0,
    ),
  };
}
