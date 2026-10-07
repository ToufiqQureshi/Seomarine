import { Link } from "@tanstack/react-router";
import { AppBanner } from "@/client/layout/AppBanner";
import { useCreditBalance } from "@/client/features/billing/useCreditBalance";
import { BILLING_ROUTE, SUBSCRIBE_ROUTE } from "@/shared/billing";

export function FreePlanBanner() {
  const { customerQuery, isFreePlan, isOutOfCredits, isLowCredits } =
    useCreditBalance();

  if (customerQuery.isLoading || !customerQuery.data) {
    return null;
  }

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
      Buy more credits
    </Link>
  );

  if (isOutOfCredits) {
    return (
      <AppBanner variant="destructive">
        You&rsquo;ve used all your credits. {creditsActionLink} to continue
        using Seomarine.
      </AppBanner>
    );
  }

  if (isLowCredits) {
    return (
      <AppBanner variant="warning">
        You&rsquo;re running low on credits. {creditsActionLink} to keep using
        Seomarine.
      </AppBanner>
    );
  }

  if (isFreePlan) {
    return (
      <AppBanner variant="info">
        We hope you&rsquo;re enjoying Seomarine!{" "}
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
