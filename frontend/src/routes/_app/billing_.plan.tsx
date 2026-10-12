import { createFileRoute } from "@tanstack/react-router";
import { BillingPlanPage } from "@/client/features/billing/plan/BillingPlanPage";

export const Route = createFileRoute("/_app/billing_/plan")({
  component: BillingPlanPage,
});
