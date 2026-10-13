import { Link, createFileRoute } from "@tanstack/react-router";
import { PageHeader } from "@/client/components/PageHeader";
import { QueryError } from "@/client/components/QueryState";
import { SkeletonPage } from "@/client/components/SkeletonPresets";
import { Button } from "@/client/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/client/components/ui/card";
import { BillingFeatureBreakdown } from "@/client/features/billing/BillingFeatureBreakdown";
import { BillingUsageChart } from "@/client/features/billing/BillingUsageChart";
import { getCustomerPaidPlan } from "@/client/features/billing/plan-detection";
import { getBillingRouteState } from "@/client/features/billing/route-state";
import { useCreditBalance } from "@/client/features/billing/useCreditBalance";
import { useCanManageBilling } from "@/client/features/team/organizationQueries";

export const Route = createFileRoute("/_app/billing")({
  component: BillingPage,
});

function BillingPage() {
  const {
    session: { data: session, isPending: isSessionPending },
    customerQuery,
    isFreePlan,
    monthlyRemaining,
    totalRemaining,
    isOutOfCredits,
    isLowCredits,
  } = useCreditBalance();
  const canManageBilling = useCanManageBilling();
  const paidPlan = getCustomerPaidPlan(customerQuery.data);
  const routeState = getBillingRouteState({
    hasSession: Boolean(session?.user?.id),
    isSessionPending,
    isCustomerLoading: customerQuery.isLoading,
    isCustomerError: customerQuery.isError,
    hasCustomerData: customerQuery.data != null,
  });

  if (routeState === "loading") return <SkeletonPage />;

  if (routeState === "error") {
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

  return (
    <div className="mx-auto box-content max-w-7xl space-y-5 px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <PageHeader title="Usage & credits" />

      {customerQuery.isError ? (
        <QueryError
          error={customerQuery.error}
          fallback="Failed to refresh billing details"
          onRetry={() => void customerQuery.refetch()}
          isRetrying={customerQuery.isFetching}
        />
      ) : null}

      <Card>
        <CardHeader>
          <CardTitle>Credits</CardTitle>
          <CardDescription>
            Your current balance and plan. Plan changes are managed in Billing.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          <div>
            <p className="text-2xl font-semibold tabular-nums">
              ${totalRemaining.toFixed(2)}{" "}
              <span className="text-sm font-normal text-muted-foreground">
                remaining
              </span>
            </p>
            {!isFreePlan ? (
              <p className="mt-1 text-xs text-muted-foreground">
                Monthly balance: ${monthlyRemaining.toFixed(2)}
              </p>
            ) : null}
          </div>

          <p className="text-sm">
            <span className="font-medium">Plan:</span>{" "}
            <span className="text-muted-foreground">
              {paidPlan?.name ?? "Free plan"}
            </span>
            {paidPlan ? (
              <span className="text-muted-foreground">
                {" "}
                &middot; ${paidPlan.monthlyCreditsUsd.toFixed(2)} monthly usage
                credits
              </span>
            ) : null}
          </p>

          {isOutOfCredits ? (
            <p className="text-sm text-destructive">
              No usage credits remain. Check your plan and billing options.
            </p>
          ) : isLowCredits ? (
            <p className="text-sm text-amber-600">
              Your usage credit balance is running low.
            </p>
          ) : null}

          <div className="space-y-2 border-t border-border pt-4">
            <Button
              variant="outline"
              nativeButton={false}
              render={<Link to="/billing/plan" />}
            >
              View plan and billing options
            </Button>
            {!canManageBilling ? (
              <p className="text-sm text-muted-foreground">
                Only the organization owner can change the plan.
              </p>
            ) : null}
          </div>
        </CardContent>
      </Card>

      <BillingUsageChart />
      <BillingFeatureBreakdown />
    </div>
  );
}
