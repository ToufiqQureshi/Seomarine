import { Link } from "@tanstack/react-router";
import { AppBanner } from "@/client/layout/AppBanner";
import {
  creditLevel,
  useBillingUsage,
} from "@/client/features/billing/usageApi";
import { BILLING_ROUTE, SUBSCRIBE_ROUTE } from "@/shared/billing";

export function FreePlanBanner() {
  const { data: usage } = useBillingUsage();
  if (!usage) return null;

  const isFreePlan = usage.plan === "free";
  const level = creditLevel(usage);
  const creditsActionLink = isFreePlan ? (
    <Link
      to={SUBSCRIBE_ROUTE}
      search={{ upgrade: true }}
      className="font-medium text-primary underline-offset-4 hover:underline"
    >
      Upgrade your plan
    </Link>
  ) : (
    <Link
      to={BILLING_ROUTE}
      className="font-medium text-primary underline-offset-4 hover:underline"
    >
      See your credits
    </Link>
  );

  if (!isFreePlan && level === "out") {
    return (
      <AppBanner variant="destructive">
        You&rsquo;ve used this month&rsquo;s credits. {creditsActionLink} to see
        when they refill.
      </AppBanner>
    );
  }

  if (!isFreePlan && level === "low") {
    return (
      <AppBanner variant="warning">
        You&rsquo;re running low on credits. {creditsActionLink}.
      </AppBanner>
    );
  }

  if (isFreePlan) {
    return (
      <AppBanner variant="info">
        We hope you&rsquo;re enjoying Uplumo!{" "}
        <Link
          to={SUBSCRIBE_ROUTE}
          search={{ upgrade: true }}
          className="font-medium text-primary underline-offset-4 hover:underline"
        >
          Upgrade anytime
        </Link>{" "}
        or{" "}
        <Link
          to="/support"
          className="font-medium text-primary underline-offset-4 hover:underline"
        >
          reach out with questions
        </Link>
        .
      </AppBanner>
    );
  }

  return null;
}
