import { z } from "zod";
import { apiRequest, shouldRetryApiError } from "@/client/lib/seomarineApi";

const statusSchema = z.object({
  plan: z.enum(["free", "pro"]),
  status: z.string(),
  currentPeriodEnd: z.iso.datetime({ offset: true }).nullable(),
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

export function createCheckout(): Promise<CheckoutSession> {
  return apiRequest("/api/v1/billing/checkout", checkoutSchema, "POST");
}

// Razorpay subscription states (plus "none" for no subscription), in words
// an owner understands. Anything else shows as sent. The Go API grants Pro
// only while a subscription is authenticated, active or pending, so every
// other state arrives with plan "free".
const STATUS_TEXT: Record<string, { label: string; detail?: string }> = {
  none: { label: "Free" },
  created: {
    label: "Awaiting payment",
    detail: "Finish checkout to start your Pro plan.",
  },
  authenticated: {
    label: "Starting",
    detail: "Payment approved. Your Pro plan starts within a few minutes.",
  },
  active: { label: "Active" },
  pending: {
    label: "Payment due",
    detail: "Your last payment didn't go through. Razorpay will try again.",
  },
  halted: {
    label: "Payment failed",
    detail:
      "Payments failed several times, so Pro is off. Update your payment method in Razorpay to turn it back on.",
  },
  paused: {
    label: "Paused",
    detail: "Your subscription is paused, so Pro is off until it resumes.",
  },
  cancelled: {
    label: "Cancelled",
    detail: "Your Pro plan was cancelled. You can upgrade again any time.",
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
