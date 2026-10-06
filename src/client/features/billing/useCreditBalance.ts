import { useCustomer } from "autumn-js/react";
import { useSession } from "@/lib/auth-client";
import { getCustomerPlanStatus } from "@/client/features/billing/plan-detection";
import {
  AUTUMN_SEO_DATA_BALANCE_FEATURE_ID,
  AUTUMN_SEO_DATA_TOPUP_BALANCE_FEATURE_ID,
  LOW_CREDITS_THRESHOLD_USD,
  autumnSeoDataCreditsToUsd,
} from "@/shared/billing";

type CustomerParams = NonNullable<Parameters<typeof useCustomer>[0]>;

/** The signed-in organization's Autumn customer, plan, and remaining credits in USD. */
export function useCreditBalance({
  expand,
}: { expand?: CustomerParams["expand"] } = {}) {
  const session = useSession();
  const customerQuery = useCustomer({
    expand,
    queryOptions: {
      enabled: Boolean(session.data?.user?.id),
    },
  });

  const balances = customerQuery.data?.balances;
  const monthlyRemaining = autumnSeoDataCreditsToUsd(
    balances?.[AUTUMN_SEO_DATA_BALANCE_FEATURE_ID]?.remaining ?? 0,
  );
  const topUpRemaining = autumnSeoDataCreditsToUsd(
    balances?.[AUTUMN_SEO_DATA_TOPUP_BALANCE_FEATURE_ID]?.remaining ?? 0,
  );
  const totalRemaining = monthlyRemaining + topUpRemaining;
  const isOutOfCredits = totalRemaining <= 0;

  return {
    session,
    customerQuery,
    isFreePlan: getCustomerPlanStatus(customerQuery.data) === "free",
    monthlyRemaining,
    topUpRemaining,
    totalRemaining,
    isOutOfCredits,
    isLowCredits: !isOutOfCredits && totalRemaining < LOW_CREDITS_THRESHOLD_USD,
  };
}
