import { createFileRoute } from "@tanstack/react-router";
import { PageHeader } from "@/client/components/PageHeader";
import { QueryError } from "@/client/components/QueryState";
import { SkeletonPage } from "@/client/components/SkeletonPresets";
import { BillingUsageChart } from "@/client/features/billing/BillingUsageChart";
import { CreditBreakdown } from "@/client/features/billing/credits/CreditBreakdown";
import { CreditsCard } from "@/client/features/billing/credits/CreditsCard";
import { useBillingUsage } from "@/client/features/billing/usageApi";
import { useCanManageBilling } from "@/client/features/team/organizationQueries";
import { useSession } from "@/lib/auth-client";

export const Route = createFileRoute("/_app/billing")({
  component: BillingPage,
});

function BillingPage() {
  const session = useSession();
  const usageQuery = useBillingUsage({
    enabled: Boolean(session.data?.user?.id),
  });
  const canManageBilling = useCanManageBilling();

  if (session.isPending || usageQuery.isLoading) return <SkeletonPage />;

  if (!usageQuery.data) {
    return (
      <div className="mx-auto box-content max-w-7xl space-y-4 px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
        <PageHeader title="Billing unavailable" />
        <QueryError
          error={usageQuery.error}
          fallback="We couldn't load your billing details right now. Please try again."
          onRetry={() => void usageQuery.refetch()}
          isRetrying={usageQuery.isFetching}
        />
      </div>
    );
  }

  return (
    <div className="mx-auto box-content max-w-7xl space-y-5 px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <PageHeader title="Usage & credits" />
      {usageQuery.isError ? (
        <QueryError
          error={usageQuery.error}
          fallback="Failed to refresh billing details"
          onRetry={() => void usageQuery.refetch()}
          isRetrying={usageQuery.isFetching}
        />
      ) : null}
      <CreditsCard
        usage={usageQuery.data}
        canManageBilling={canManageBilling}
      />
      <CreditBreakdown usage={usageQuery.data} />
      <BillingUsageChart />
    </div>
  );
}
