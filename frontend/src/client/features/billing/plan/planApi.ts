import { z } from "zod";
import { apiRequest, shouldRetryApiError } from "@/client/lib/uplumoApi";

const statusSchema = z.object({
  plan: z.enum(["free", "basic", "pro", "agency"]),
  status: z.string(),
  currentPeriodEnd: z.iso.datetime({ offset: true }).nullable(),
  // True once the owner cancelled: Pro runs until currentPeriodEnd, then stops.
  cancelAtPeriodEnd: z.boolean(),
  // True for Uplumo's own workspaces: every feature, nothing to pay.
  unlimited: z.boolean(),
  paymentsConfigured: z.boolean(),
  availablePlans: z.array(z.enum(["basic", "pro", "agency"])),
  usage: z.object({
    sites: z.object({
      used: z.number().int().nonnegative(),
      limit: z.number().int().nonnegative(),
      exceeded: z.boolean(),
    }),
    members: z.object({
      used: z.number().int().nonnegative(),
      limit: z.number().int().nonnegative(),
      exceeded: z.boolean(),
    }),
    trackedKeywords: z.object({
      used: z.number().int().nonnegative(),
      limit: z.number().int().nonnegative(),
      exceeded: z.boolean(),
    }),
  }),
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

/**
 * Razorpay tells the browser a payment succeeded about a second before its
 * webhook reaches the Go API, and only the webhook changes the plan. Reads
 * the status until the paid plan shows (or the attempts run out) and returns
 * the last status read.
 */
export async function waitForPaidPlan(
  delayMs = 1500,
  attempts = 20,
): Promise<BillingStatus> {
  let status = await billingStatusQueryOptions.queryFn();
  for (let i = 1; i < attempts && status.plan === "free"; i++) {
    await new Promise((resolve) => setTimeout(resolve, delayMs));
    status = await billingStatusQueryOptions.queryFn();
  }
  return status;
}

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

const planSchema = z.object({
  id: z.enum(["free", "basic", "pro", "agency"]),
  name: z.string().min(1),
  priceInr: z.number().int().nonnegative(),
  summary: z.string(),
  highlights: z.array(z.string()),
});
export type BillingPlan = z.infer<typeof planSchema>;

// Prices and what each plan includes come from the Go plan catalog, the same
// list the public pricing page and every plan check read. Never copy a price
// into this app.
export const billingPlansQueryOptions = {
  queryKey: ["goBillingPlans"],
  queryFn: async () =>
    (
      await apiRequest(
        "/api/v1/billing/plans",
        z.object({ plans: z.array(planSchema) }),
      )
    ).plans,
  retry: shouldRetryApiError,
  staleTime: Infinity,
};

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
