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

// Razorpay subscription states (plus "free" for no subscription), in words
// an owner understands. Anything else shows as sent.
const STATUS_TEXT: Record<string, { label: string; detail?: string }> = {
  free: { label: "Free" },
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
    label: "Paused",
    detail:
      "Payments failed several times. Update your payment method in Razorpay to continue Pro.",
  },
  cancelled: {
    label: "Cancelled",
    detail: "You keep Pro until the end of the current period.",
  },
  completed: { label: "Ended" },
  expired: { label: "Expired" },
};

export function describeStatus(status: string): {
  label: string;
  detail?: string;
} {
  return STATUS_TEXT[status] ?? { label: status };
}
