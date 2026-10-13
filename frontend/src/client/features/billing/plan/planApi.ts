import { z } from "zod";
import { apiRequest, shouldRetryApiError } from "@/client/lib/uplumoApi";

const statusSchema = z.object({
  plan: z.enum(["free", "basic", "pro", "agency"]),
  status: z.string(),
  currentPeriodEnd: z.iso.datetime({ offset: true }).nullable(),
  // True once the owner cancelled: Pro runs until currentPeriodEnd, then stops.
  cancelAtPeriodEnd: z.boolean(),
  paymentsConfigured: z.boolean(),
  availablePlans: z.array(z.enum(["basic", "pro", "agency"])),
});
export type BillingStatus = z.infer<typeof statusSchema>;

const checkoutSchema = z.object({
  subscriptionId: z.string().min(1),
  keyId: z.string().min(1),
});
type CheckoutSession = z.infer<typeof checkoutSchema>;

export const billingStatusQueryOptions = {
  queryKey: ["goBillingStatus"],
  queryFn: () => apiRequest("/api/v1/billing/status", statusSchema),
  retry: shouldRetryApiError,
};

export function createCheckout(
  plan: "basic" | "pro" | "agency",
): Promise<CheckoutSession> {
  return apiRequest("/api/v1/billing/checkout", checkoutSchema, "POST", {
    plan,
  });
}

/** Cancels the subscription and returns the plan that results. */
export function cancelSubscription(): Promise<BillingStatus> {
  return apiRequest("/api/v1/billing/cancel", statusSchema, "POST");
}

// What Pro costs a month, in rupees. It must match the Razorpay plan the Go
// API sells and ProPriceINR in backend/internal/site, which the public
// pricing page prints.
export const BASIC_PRICE_INR = 899;
export const PRO_PRICE_INR = 1899;
export const AGENCY_PRICE_INR = 4999;

// Razorpay subscription states (plus "none" for no subscription), in words
// an owner understands. Anything else shows as sent. The Go API grants Pro
// only while a subscription is authenticated, active or pending, so every
// other state arrives with plan "free".
const STATUS_TEXT: Record<string, { label: string; detail?: string }> = {
  none: { label: "Free" },
  created: {
    label: "Awaiting payment",
    detail: "Finish checkout to activate your selected plan.",
  },
  authenticated: {
    label: "Starting",
    detail: "Payment approved. Your plan starts within a few minutes.",
  },
  active: { label: "Active" },
  pending: {
    label: "Payment due",
    detail: "Your last payment didn't go through. Razorpay will try again.",
  },
  halted: {
    label: "Payment failed",
    detail:
      "Payments failed several times, so your paid plan is off. Update your payment method in Razorpay to turn it back on.",
  },
  paused: {
    label: "Paused",
    detail:
      "Your subscription is paused, so your paid plan is off until it resumes.",
  },
  cancelled: {
    label: "Cancelled",
    detail: "Your paid plan was cancelled. You can subscribe again any time.",
  },
  completed: { label: "Ended" },
  expired: { label: "Expired" },
};

// States of a subscription that can still charge the customer. The Go API
// refuses a second checkout for these, so the page offers no Upgrade button.
const LIVE_STATUSES = new Set([
  "authenticated",
  "active",
  "pending",
  "halted",
  "paused",
]);

export function describeStatus(status: string): {
  label: string;
  detail?: string;
  canUpgrade: boolean;
} {
  return {
    ...(STATUS_TEXT[status] ?? { label: status }),
    canUpgrade: !LIVE_STATUSES.has(status),
  };
}
