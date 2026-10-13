import {
  AUTUMN_YC_CHECKOUT_SESSION_PARAMS,
  AUTUMN_YC_PLAN_ID,
} from "@/shared/billing";

// What a checkout page advertises before the customer holds the plan. Once
// they do, the Autumn customer is the source of truth (see plan-detection.ts),
// so these numbers only need to match the Autumn plan they sell.
export type PlanOffer = {
  planId: string;
  name: string;
  // The monthly price as the customer reads it, currency included.
  price: string;
  monthlyCreditsUsd: number;
  checkoutSessionParams: Record<string, unknown>;
};

export const YC_PLAN_OFFER: PlanOffer = {
  planId: AUTUMN_YC_PLAN_ID,
  name: "YC Plan",
  price: "$50",
  monthlyCreditsUsd: 50,
  checkoutSessionParams: AUTUMN_YC_CHECKOUT_SESSION_PARAMS,
};

export function monthlyCreditsFeature(offer: PlanOffer) {
  return `Includes $${offer.monthlyCreditsUsd.toFixed(2)} of Usage Credits each month`;
}
