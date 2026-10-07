import { createFileRoute, notFound } from "@tanstack/react-router";
import { useState } from "react";
import { ErrorState } from "@/client/components/ErrorState";
import { PageHeader } from "@/client/components/PageHeader";
import { QueryError } from "@/client/components/QueryState";
import { SkeletonPage } from "@/client/components/SkeletonPresets";
import { Button } from "@/client/components/ui/button";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/client/components/ui/card";
import { Field, FieldError } from "@/client/components/ui/field";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/client/components/ui/input-group";
import { isHostedClientAuthMode } from "@/lib/auth-mode";
import { useCanManageBilling } from "@/client/features/team/organizationQueries";
import { captureClientEvent } from "@/client/lib/posthog";
import { getStandardErrorMessage } from "@/client/lib/error-messages";
import { buildCheckoutSuccessUrl } from "@/client/features/billing/checkout-url";
import { BillingUsageChart } from "@/client/features/billing/BillingUsageChart";
import { BillingFeatureBreakdown } from "@/client/features/billing/BillingFeatureBreakdown";
import { parseTopUpAmount } from "@/client/features/billing/HostedBillingContentUtils";
import { getBillingRouteState } from "@/client/features/billing/route-state";
import { getCustomerPaidPlan } from "@/client/features/billing/plan-detection";
import { useCreditBalance } from "@/client/features/billing/useCreditBalance";
import {
  BASE_PLAN_OFFER,
  monthlyCreditsFeature,
} from "@/client/features/billing/plan-offers";
import {
  AUTUMN_CHECKOUT_SESSION_PARAMS,
  BILLING_ROUTE,
  AUTUMN_SEO_DATA_CREDITS_PER_USD,
  AUTUMN_SEO_DATA_TOP_UP_PLAN_ID,
  AUTUMN_SEO_DATA_TOPUP_BALANCE_FEATURE_ID,
} from "@/shared/billing";

export const Route = createFileRoute("/_app/billing")({
  beforeLoad: () => {
    if (!isHostedClientAuthMode()) {
      throw notFound();
    }
  },
  component: BillingPage,
});

type BillingAction = "plan" | "topUp";

function BillingPage() {
  const [topUpAmount, setTopUpAmount] = useState("20");
  const [isPending, setIsPending] = useState(false);
  const [error, setError] = useState<{
    action: BillingAction;
    message: string;
  } | null>(null);

  const {
    session: { data: session, isPending: isSessionPending },
    customerQuery,
    isFreePlan,
    monthlyRemaining,
    topUpRemaining,
    totalRemaining,
    isOutOfCredits,
    isLowCredits,
  } = useCreditBalance({
    // Expanded so the plan card can show the subscribed plan's name.
    expand: ["subscriptions.plan"],
  });

  // Subscription changes are owner-only; other members see balances but are
  // pointed at the owner instead of checkout (the server enforces this too).
  const canManageBilling = useCanManageBilling();

  const paidPlan = getCustomerPaidPlan(customerQuery.data);
  const billingRouteState = getBillingRouteState({
    hasSession: Boolean(session?.user?.id),
    isSessionPending,
    isCustomerLoading: customerQuery.isLoading,
    isCustomerError: customerQuery.isError,
    hasCustomerData: customerQuery.data != null,
  });

  const { isValid: isValidTopUp, parsed: parsedTopUpAmount } =
    parseTopUpAmount(topUpAmount);
  const showTopUpError = topUpAmount.trim() !== "" && !isValidTopUp;

  if (billingRouteState === "loading") {
    return <SkeletonPage />;
  }

  if (billingRouteState === "error") {
    return (
      <div className="mx-auto box-content max-w-7xl space-y-4 px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
        <PageHeader title="Billing unavailable" />
        <QueryError
          error={customerQuery.error}
          fallback="We couldn't load your billing details right now. Please try again."
          onRetry={() => void customerQuery.refetch()}
          isRetrying={customerQuery.isFetching}
        />
      </div>
    );
  }

  function startUpgradeCheckout() {
    captureClientEvent("billing:checkout_start");
    return customerQuery.attach({
      planId: BASE_PLAN_OFFER.planId,
      redirectMode: "always",
      successUrl: buildCheckoutSuccessUrl(BILLING_ROUTE),
      checkoutSessionParams: BASE_PLAN_OFFER.checkoutSessionParams,
    });
  }

  async function runAction(
    action: BillingAction,
    callback: () => Promise<unknown>,
    fallbackMessage: string,
  ) {
    setError(null);
    setIsPending(true);
    try {
      await callback();
      await customerQuery.refetch();
    } catch (err) {
      setError({
        action,
        message: getStandardErrorMessage(err, fallbackMessage),
      });
    } finally {
      setIsPending(false);
    }
  }

  function actionError(action: BillingAction) {
    return error?.action === action ? (
      <ErrorState message={error.message} />
    ) : null;
  }

  if (isPending) {
    return (
      <div className="flex h-full items-center justify-center">
        <p className="text-sm text-muted-foreground">
          Redirecting to Stripe...
        </p>
      </div>
    );
  }

  return (
    <div className="mx-auto box-content max-w-7xl space-y-5 px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <PageHeader title="Billing" />

      {customerQuery.isError ? (
        <QueryError
          error={customerQuery.error}
          fallback="Failed to refresh billing details"
          onRetry={() => void customerQuery.refetch()}
          isRetrying={customerQuery.isFetching}
        />
      ) : null}

      <div className="grid gap-5 md:grid-cols-2">
        {/* Subscription card */}
        <Card>
          <CardContent className="flex h-full flex-col justify-between gap-4">
            <div>
              <div className="text-2xl font-semibold tabular-nums">
                ${totalRemaining.toFixed(2)}{" "}
                <span className="text-sm font-normal text-muted-foreground">
                  remaining
                </span>
              </div>
              {!isFreePlan ? (
                <div className="mt-1 flex gap-3 text-xs text-muted-foreground">
                  <span className="tabular-nums">
                    Monthly ${monthlyRemaining.toFixed(2)}
                  </span>
                  <span>&middot;</span>
                  <span className="tabular-nums">
                    Top-ups ${topUpRemaining.toFixed(2)}
                  </span>
                </div>
              ) : null}
              {isOutOfCredits ? (
                <p className="mt-2 text-xs text-destructive">
                  You&rsquo;ve used all your credits.{" "}
                  {isFreePlan
                    ? "Upgrade your plan to continue."
                    : "Buy more credits below to continue."}
                </p>
              ) : isLowCredits ? (
                <p className="mt-2 text-xs text-amber-600">
                  You&rsquo;re running low on credits.{" "}
                  {isFreePlan
                    ? `Upgrade to get $${BASE_PLAN_OFFER.monthlyCreditsUsd}/month.`
                    : "Buy more credits below."}
                </p>
              ) : null}
            </div>

            <div className="text-sm">
              <span className="font-medium">Plan</span>{" "}
              <span className="text-muted-foreground">
                {paidPlan?.name ?? "Free Plan"}
              </span>
              {paidPlan ? (
                <span className="text-muted-foreground">
                  {" "}
                  &middot; ${paidPlan.monthlyCreditsUsd.toFixed(2)} of Usage
                  Credits each month
                </span>
              ) : null}
            </div>

            {!canManageBilling ? (
              <p className="border-t border-border pt-3 text-sm text-muted-foreground">
                Only the organization owner can change the plan or buy credits.
                Ask them if you need more.
              </p>
            ) : isFreePlan ? (
              <div className="space-y-3 border-t border-border pt-3">
                <div className="flex items-baseline justify-between gap-4">
                  <span className="text-sm font-medium">
                    {BASE_PLAN_OFFER.name}
                  </span>
                  <span className="text-sm font-medium tabular-nums">
                    ${BASE_PLAN_OFFER.priceUsd}/month
                  </span>
                </div>
                <ul className="space-y-1.5">
                  {[
                    "Access to all Seomarine features",
                    monthlyCreditsFeature(BASE_PLAN_OFFER),
                  ].map((item) => (
                    <li
                      key={item}
                      className="flex gap-2 text-xs text-muted-foreground"
                    >
                      <span className="mt-[1px] shrink-0 text-muted-foreground/60">
                        &mdash;
                      </span>
                      {item}
                    </li>
                  ))}
                </ul>
                <Button
                  variant="secondary"
                  className="w-full"
                  onClick={() =>
                    void runAction(
                      "plan",
                      startUpgradeCheckout,
                      "We couldn't start the checkout. Please try again.",
                    )
                  }
                >
                  Upgrade Plan
                </Button>
                {actionError("plan")}
              </div>
            ) : (
              <div className="space-y-3">
                <Button
                  variant="secondary"
                  className="w-full"
                  onClick={() =>
                    void runAction(
                      "plan",
                      () =>
                        customerQuery.openCustomerPortal({
                          returnUrl: window.location.href,
                        }),
                      "We couldn't open the billing portal. Please try again.",
                    )
                  }
                >
                  Manage subscription
                </Button>
                {actionError("plan")}
              </div>
            )}
          </CardContent>
        </Card>

        {/* Buy credits card — paid plan only, owner-only */}
        {!isFreePlan && canManageBilling ? (
          <Card>
            <CardHeader>
              <CardTitle>Buy credits</CardTitle>
              <CardDescription>
                Top-up credits never expire and are used after your monthly
                credits.
              </CardDescription>
            </CardHeader>
            <CardContent className="space-y-3">
              <Field data-invalid={showTopUpError}>
                <InputGroup>
                  <InputGroupAddon>$</InputGroupAddon>
                  <InputGroupInput
                    type="number"
                    min={10}
                    max={99}
                    step={1}
                    inputMode="numeric"
                    aria-label="Top-up amount in USD"
                    aria-invalid={showTopUpError}
                    value={topUpAmount}
                    onChange={(e) => setTopUpAmount(e.target.value)}
                  />
                </InputGroup>
                {showTopUpError ? (
                  <FieldError>Enter between $10–$99.</FieldError>
                ) : null}
              </Field>

              <Button
                variant="secondary"
                className="w-full"
                disabled={!isValidTopUp}
                onClick={() =>
                  void runAction(
                    "topUp",
                    () =>
                      customerQuery.attach({
                        planId: AUTUMN_SEO_DATA_TOP_UP_PLAN_ID,
                        redirectMode: "always",
                        successUrl: window.location.href,
                        checkoutSessionParams: AUTUMN_CHECKOUT_SESSION_PARAMS,
                        featureQuantities: [
                          {
                            featureId: AUTUMN_SEO_DATA_TOPUP_BALANCE_FEATURE_ID,
                            quantity: Math.round(
                              parsedTopUpAmount *
                                AUTUMN_SEO_DATA_CREDITS_PER_USD,
                            ),
                          },
                        ],
                      }),
                    "We couldn't start the checkout. Please try again.",
                  )
                }
              >
                Buy credits
              </Button>
              {actionError("topUp")}
            </CardContent>
          </Card>
        ) : null}
      </div>

      <BillingUsageChart />

      <BillingFeatureBreakdown />

      <p className="text-xs text-muted-foreground">
        Billing is powered by Stripe.
      </p>
    </div>
  );
}
