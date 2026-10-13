import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/client/components/ui/card";
import { formatCredits } from "@/client/features/billing/credits/format";
import {
  creditFeatureLabel,
  type BillingUsage,
} from "@/client/features/billing/usageApi";

/** Where this month's credits went, and what each action costs. */
export function CreditBreakdown({ usage }: { usage: BillingUsage }) {
  const total = usage.spent.reduce((sum, row) => sum + row.credits, 0);

  return (
    <div className="grid gap-5 lg:grid-cols-2">
      <Card>
        <CardHeader>
          <CardTitle>Where your credits went</CardTitle>
          <CardDescription>This billing month, by feature.</CardDescription>
        </CardHeader>
        <CardContent>
          {usage.spent.length === 0 ? (
            <p className="text-sm text-muted-foreground">
              No credits used yet this month.
            </p>
          ) : (
            <ul className="space-y-3">
              {usage.spent.map((row) => (
                <li key={row.feature} className="space-y-1">
                  <div className="flex items-baseline justify-between gap-3 text-sm">
                    <span>{creditFeatureLabel(row.feature)}</span>
                    <span className="tabular-nums text-muted-foreground">
                      {formatCredits(row.credits)}
                    </span>
                  </div>
                  <div
                    className="h-1.5 rounded-full bg-primary/70"
                    style={{
                      width: `${Math.max(2, (row.credits / total) * 100)}%`,
                    }}
                    aria-hidden="true"
                  />
                </li>
              ))}
            </ul>
          )}
        </CardContent>
      </Card>
      <Card>
        <CardHeader>
          <CardTitle>What things cost</CardTitle>
          <CardDescription>
            Typical credits per action. You are charged the real cost of each
            request, which can be a little more or less.
          </CardDescription>
        </CardHeader>
        <CardContent>
          <ul className="divide-y divide-border text-sm">
            {usage.costs.map((cost) => (
              <li
                key={cost.feature}
                className="flex items-baseline justify-between gap-3 py-2"
              >
                <span>
                  {creditFeatureLabel(cost.feature)}
                  <span className="block text-xs text-muted-foreground">
                    per {cost.unit}
                  </span>
                </span>
                <span className="shrink-0 tabular-nums">
                  about {formatCredits(cost.credits)}
                </span>
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>
    </div>
  );
}
