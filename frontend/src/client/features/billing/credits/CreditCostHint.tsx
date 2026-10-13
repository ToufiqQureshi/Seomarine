import { Link } from "@tanstack/react-router";
import { cn } from "cn";
import { formatCredits } from "@/client/features/billing/credits/format";
import {
  creditLevel,
  useBillingUsage,
  type CreditFeature,
} from "@/client/features/billing/usageApi";
import { BILLING_ROUTE } from "@/shared/billing";

/**
 * A one-line note next to a paid action: about what it costs and how many
 * credits are left. Shows nothing where credits are not charged, and nothing
 * while loading, so it never blocks the action.
 */
export function CreditCostHint({
  feature,
  className,
}: {
  feature: CreditFeature;
  className?: string;
}) {
  const { data: usage } = useBillingUsage();
  if (!usage) return null;
  const level = creditLevel(usage);
  if (level === "not_charged") return null;
  const cost = usage.costs.find((c) => c.feature === feature);

  if (level === "out") {
    return (
      <p className={cn("text-xs text-destructive", className)}>
        No credits left.{" "}
        <Link to={BILLING_ROUTE} className="underline underline-offset-4">
          See your credits
        </Link>
      </p>
    );
  }
  return (
    <p className={cn("text-xs text-muted-foreground", className)}>
      {cost
        ? `About ${formatCredits(cost.credits)} credits per ${cost.unit} · `
        : null}
      {formatCredits(usage.balanceCredits)} left
    </p>
  );
}
