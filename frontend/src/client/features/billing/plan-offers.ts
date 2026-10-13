import {
  AUTUMN_CHECKOUT_SESSION_PARAMS,
  AUTUMN_PAID_PLAN_ID,
  AUTUMN_YC_CHECKOUT_SESSION_PARAMS,
  AUTUMN_YC_PLAN_ID,
} from "@/shared/billing";
import { PRO_PRICE_INR } from "@/client/features/billing/plan/planApi";

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

export const BASE_PLAN_OFFER: PlanOffer = {
  planId: AUTUMN_PAID_PLAN_ID,
  name: "Uplumo Pro",
  price: new Intl.NumberFormat("en-IN", {
    style: "currency",
    currency: "INR",
    maximumFractionDigits: 0,
  }).format(PRO_PRICE_INR),
  monthlyCreditsUsd: 10,
  checkoutSessionParams: AUTUMN_CHECKOUT_SESSION_PARAMS,
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
