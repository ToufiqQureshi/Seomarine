import { createFileRoute } from "@tanstack/react-router";
import { CreditCard } from "lucide-react";
import { EmptyState } from "@/client/components/EmptyState";
import { PageHeader } from "@/client/components/PageHeader";

export const Route = createFileRoute("/_app/billing_/plan")({
  component: BillingPlanRoute,
});

// Placeholder until plan checkout (served by the Go API) lands.
function BillingPlanRoute() {
  return (
    <div className="h-full overflow-auto px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <div className="mx-auto max-w-7xl space-y-8">
        <PageHeader
          title="Billing"
          description="Your plan, what it costs and when it renews."
        />
        <EmptyState
          variant="card"
          size="lg"
          icon={CreditCard}
          title="Plans are on their way"
          description="Soon you can see your plan, upgrade in one step and cancel in one click from this page."
        />
      </div>
    </div>
  );
}
