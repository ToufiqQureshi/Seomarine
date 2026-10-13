import { Link } from "@tanstack/react-router";
import { Button } from "@/client/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/client/components/ui/card";
import { Progress } from "@/client/components/ui/progress";
import { formatCredits } from "@/client/features/billing/credits/format";
import {
  creditLevel,
  type BillingUsage,
} from "@/client/features/billing/usageApi";

const refillDate = new Intl.DateTimeFormat("en-IN", {
  day: "numeric",
  month: "short",
});

/** Credits left, the monthly allowance, and when they refill. */
export function CreditsCard({
  usage,
  canManageBilling,
}: {
  usage: BillingUsage;
  canManageBilling: boolean;
}) {
  const level = creditLevel(usage);
  const share =
    usage.monthlyCredits > 0
      ? Math.min(100, (usage.balanceCredits / usage.monthlyCredits) * 100)
      : 0;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Credits</CardTitle>
        <CardDescription>
          Every live data lookup and AI answer uses credits. Your plan refills
          them every billing month.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {level === "not_charged" ? (
          <p className="text-sm text-muted-foreground">
            This workspace is not charged credits.
          </p>
        ) : (
          <>
            <div>
              <p className="text-2xl font-semibold tabular-nums">
                {formatCredits(usage.balanceCredits)}{" "}
                <span className="text-sm font-normal text-muted-foreground">
                  credits left
                </span>
              </p>
              {usage.monthlyCredits > 0 ? (
                <Progress
                  className="mt-3"
                  value={share}
                  aria-label={`${formatCredits(usage.balanceCredits)} of ${formatCredits(usage.monthlyCredits)} monthly credits left`}
                />
              ) : null}
              <p className="mt-2 text-xs text-muted-foreground">
                {usage.monthlyCredits > 0
                  ? `Your plan adds ${formatCredits(usage.monthlyCredits)} credits every month`
                  : "The Free plan has no credits. Upgrade to use live data and AI."}
                {usage.periodEnd
                  ? ` · next refill ${refillDate.format(new Date(usage.periodEnd))}`
                  : null}
              </p>
            </div>
            {level === "out" ? (
              <p className="text-sm text-destructive">
                No credits left. Live data and AI are paused until your credits
                refill, or upgrade your plan for more.
              </p>
            ) : level === "low" ? (
              <p className="text-sm text-amber-600">
                Your credits are running low.
              </p>
            ) : null}
          </>
        )}
        <div className="space-y-2 border-t border-border pt-4">
          <Button
            variant="outline"
            nativeButton={false}
            render={<Link to="/billing/plan" />}
          >
            View plans
          </Button>
          {!canManageBilling ? (
            <p className="text-sm text-muted-foreground">
              Only the organization owner can change the plan.
            </p>
          ) : null}
        </div>
      </CardContent>
    </Card>
  );
}
