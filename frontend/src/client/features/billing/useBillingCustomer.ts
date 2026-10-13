import { useQuery } from "@tanstack/react-query";
import {
  getBillingCredits,
  type BillingCredits,
} from "@/client/features/billing/billingApi";
import { createCheckout } from "@/client/features/billing/plan/planApi";
import { openRazorpayCheckout } from "@/client/features/billing/plan/razorpay";
import { SUPPORT_EMAIL } from "@/client/lib/support";
import {
  AUTUMN_MANAGED_ACCESS_FEATURE_ID,
  AUTUMN_PAID_PLAN_FEATURE_ID,
  AUTUMN_SEO_DATA_BALANCE_FEATURE_ID,
  AUTUMN_SEO_DATA_TOPUP_BALANCE_FEATURE_ID,
  AUTUMN_SEO_DATA_TOP_UP_PLAN_ID,
} from "@/shared/billing";

/** The plan and credit shape the billing screens read. */
type BillingCustomer = {
  paymentsConfigured: boolean;
  flags: Record<string, { planId: string | null } | undefined>;
  subscriptions: Array<{
    planId: string;
    plan: { name: string };
    pastDue: boolean;
  }>;
  balances: Record<string, { remaining: number; granted: number }>;
};

function toCustomer(credits: BillingCredits): BillingCustomer {
  const paid = credits.plan !== "free";
  return {
    paymentsConfigured: credits.paymentsConfigured,
    flags: paid
      ? { [AUTUMN_PAID_PLAN_FEATURE_ID]: { planId: credits.plan } }
      : {},
    subscriptions: paid
      ? [
          {
            planId: credits.plan,
            plan: { name: "Uplumo Pro" },
            pastDue:
              credits.status === "halted" || credits.status === "pending",
          },
        ]
      : [],
    balances: {
      [AUTUMN_SEO_DATA_BALANCE_FEATURE_ID]: {
        remaining: credits.balanceCredits,
        granted: credits.monthlyGrantCredits,
      },
      [AUTUMN_SEO_DATA_TOPUP_BALANCE_FEATURE_ID]: { remaining: 0, granted: 0 },
    },
  };
}

// Autumn-era callers also pass checkout options Razorpay has no use for.
type AttachOptions = { planId: string; successUrl?: string } & Record<
  string,
  unknown
>;

/**
 * The signed-in organization's plan and credits from the Go billing API, in
 * the shape the billing screens already read. `attach` opens Razorpay
 * Checkout for the Pro subscription and resolves once the payment window
 * hands control back; the success URL then polls until the webhook lands.
 */
export function useBillingCustomer({
  enabled = true,
}: { enabled?: boolean } = {}) {
  const query = useQuery({
    queryKey: ["billing", "credits"],
    queryFn: getBillingCredits,
    enabled,
    staleTime: 30_000,
  });
  const data = query.data ? toCustomer(query.data) : undefined;

  async function attach({ planId, successUrl }: AttachOptions) {
    if (query.data && !query.data.paymentsConfigured) {
      throw new Error(
        "Online payments are temporarily unavailable. Contact support.",
      );
    }
    if (planId === AUTUMN_SEO_DATA_TOP_UP_PLAN_ID) {
      throw new Error("Credit top-ups are not available yet.");
    }
    const checkout = await createCheckout("pro");
    const paid = await openRazorpayCheckout(checkout);
    if (!paid) throw new Error("The payment window was closed before paying.");
    if (successUrl) window.location.assign(successUrl);
  }

  return {
    data,
    isLoading: query.isLoading,
    isError: query.isError,
    error: query.error,
    isFetching: query.isFetching,
    refetch: query.refetch,
    check: ({ featureId }: { featureId: string }) => ({
      allowed: featureId === AUTUMN_MANAGED_ACCESS_FEATURE_ID,
    }),
    attach,
    // Razorpay has no hosted customer portal; changes go through support.
    openCustomerPortal: (_options?: { returnUrl?: string }) => {
      window.location.assign(`mailto:${SUPPORT_EMAIL}?subject=Billing`);
      return Promise.resolve();
    },
  };
}
