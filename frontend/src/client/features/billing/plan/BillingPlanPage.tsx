import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { Check } from "lucide-react";
import { toast } from "sonner";
import { ErrorState } from "@/client/components/ErrorState";
import { PageHeader } from "@/client/components/PageHeader";
import { QueryError } from "@/client/components/QueryState";
import { SkeletonCard } from "@/client/components/SkeletonPresets";
import { Badge } from "@/client/components/ui/badge";
import { Button } from "@/client/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "@/client/components/ui/card";
import { useCanManageBilling } from "@/client/features/team/organizationQueries";
import { getStandardErrorMessage } from "@/client/lib/error-messages";
import { ApiError } from "@/client/lib/uplumoApi";
import {
  AGENCY_PRICE_INR,
  BASIC_PRICE_INR,
  PRO_PRICE_INR,
  billingStatusQueryOptions,
  cancelSubscription,
  createCheckout,
  describeStatus,
  type BillingStatus,
} from "./planApi";
import { openRazorpayCheckout } from "./razorpay";

const inr = new Intl.NumberFormat("en-IN", {
  style: "currency",
  currency: "INR",
  maximumFractionDigits: 0,
});

const PAID_PLANS = [
  {
    id: "basic" as const,
    name: "Basic",
    price: BASIC_PRICE_INR,
    sites: "1 website",
    features: [
      "Analytics and AI referral traffic",
      "Google Search Console and GA4 connections",
      "Search queries, pages and performance trends",
      "Saved keywords and project context",
      "No paid SEO-data or AI/LLM provider calls",
    ],
  },
  {
    id: "pro" as const,
    name: "Pro",
    price: PRO_PRICE_INR,
    sites: "Up to 5 websites",
    features: [
      "Everything in Basic",
      "Keyword research, rank tracking, domain and backlink data",
      "Technical site audits with rendered-page analysis",
      "AI visibility across ChatGPT, Perplexity, Gemini, AI Overviews and Claude",
      "$3 monthly credits for AI writing and rendered-page audits",
    ],
  },
  {
    id: "agency" as const,
    name: "Agency",
    price: AGENCY_PRICE_INR,
    sites: "Up to 25 websites",
    features: [
      "Everything in Pro",
      "White-label reports with your agency branding",
      "$10 monthly credits for AI writing and rendered-page audits",
    ],
  },
];

export function BillingPlanPage() {
  const statusQuery = useQuery(billingStatusQueryOptions);
  const canManageBilling = useCanManageBilling();

  return (
    <div className="h-full overflow-auto px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <div className="mx-auto max-w-6xl space-y-6">
        <PageHeader
          title="Plan and billing"
          description="Compare what each plan includes and manage your workspace subscription."
        />
        {statusQuery.isPending ? (
          <SkeletonCard />
        ) : statusQuery.isError ? (
          <StatusError
            error={statusQuery.error}
            onRetry={() => void statusQuery.refetch()}
            isRetrying={statusQuery.isFetching}
          />
        ) : (
          <>
            <CurrentPlanCard status={statusQuery.data} />
            <section aria-labelledby="available-plans" className="space-y-4">
              <div>
                <h2 id="available-plans" className="text-xl font-semibold">
                  Compare paid plans
                </h2>
                <p className="text-sm text-muted-foreground">
                  Prices are monthly per workspace. Usage credits apply only to
                  credit-metered AI writing and rendered audits.
                </p>
              </div>
              <div className="grid gap-4 lg:grid-cols-3">
                {PAID_PLANS.map((plan) => (
                  <PlanOption
                    key={plan.id}
                    plan={plan}
                    status={statusQuery.data}
                    canManageBilling={canManageBilling}
                  />
                ))}
              </div>
            </section>
          </>
        )}
      </div>
    </div>
  );
}

function StatusError({
  error,
  onRetry,
  isRetrying,
}: {
  error: unknown;
  onRetry: () => void;
  isRetrying: boolean;
}) {
  if (error instanceof ApiError && error.status === 401) {
    return (
      <ErrorState
        message={error.message}
        action={
          <Button
            variant="outline"
            size="sm"
            nativeButton={false}
            render={<Link to="/sign-in" />}
          >
            Sign in
          </Button>
        }
      />
    );
  }
  return (
    <QueryError
      error={error}
      fallback="We couldn't load your plan."
      onRetry={onRetry}
      isRetrying={isRetrying}
    />
  );
}

function CurrentPlanCard({ status }: { status: BillingStatus }) {
  const planName =
    status.plan === "free"
      ? "Free"
      : (PAID_PLANS.find((plan) => plan.id === status.plan)?.name ??
        status.plan);
  const { label, detail, canUpgrade } = describeStatus(status.status);
  const renews = status.currentPeriodEnd
    ? new Date(status.currentPeriodEnd).toLocaleDateString("en-IN", {
        day: "numeric",
        month: "long",
        year: "numeric",
      })
    : null;
  const isPaid = status.plan !== "free";
  const description = status.cancelAtPeriodEnd
    ? `Cancelled. ${planName} stays active until ${renews ?? "the end of your paid term"}; you will not be charged again.`
    : isPaid
      ? renews
        ? `${planName} renews on ${renews}.`
        : `Your ${planName} plan is active.`
      : "You are using the free plan.";

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          {planName} plan
          <Badge variant={status.status === "active" ? "success" : "soft"}>
            {label}
          </Badge>
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      {detail ? (
        <CardContent>
          <p className="text-sm">{detail}</p>
        </CardContent>
      ) : null}
      {isPaid && !status.cancelAtPeriodEnd && !canUpgrade ? (
        <CardFooter>
          <CancelButton
            planName={planName}
            keepsPlan={status.status === "active"}
          />
        </CardFooter>
      ) : null}
    </Card>
  );
}

function PlanOption({
  plan,
  status,
  canManageBilling,
}: {
  plan: (typeof PAID_PLANS)[number];
  status: BillingStatus;
  canManageBilling: boolean;
}) {
  const isCurrent = status.plan === plan.id;
  const hasLivePaidSubscription =
    status.plan !== "free" && !describeStatus(status.status).canUpgrade;
  const available = status.availablePlans.includes(plan.id);

  return (
    <Card className={plan.id === "pro" ? "border-primary" : undefined}>
      <CardHeader>
        <CardTitle className="flex items-center justify-between gap-2">
          {plan.name}
          {isCurrent ? <Badge variant="success">Current plan</Badge> : null}
        </CardTitle>
        <CardDescription>{plan.sites}</CardDescription>
        <p className="text-2xl font-semibold">
          {inr.format(plan.price)}
          <span className="text-sm font-normal text-muted-foreground">
            /month
          </span>
        </p>
      </CardHeader>
      <CardContent>
        <ul className="space-y-2 text-sm">
          {plan.features.map((feature) => (
            <li key={feature} className="flex gap-2">
              <Check className="mt-0.5 size-4 shrink-0 text-primary" />
              <span>{feature}</span>
            </li>
          ))}
        </ul>
      </CardContent>
      <CardFooter className="flex-col items-stretch gap-2">
        {isCurrent ? (
          <p role="status" className="text-sm text-muted-foreground">
            This workspace is on this plan.
          </p>
        ) : hasLivePaidSubscription ? (
          <p className="text-sm text-muted-foreground">
            To change an active subscription, contact support.
          </p>
        ) : !canManageBilling ? (
          <p className="text-sm text-muted-foreground">
            Only a workspace owner or admin can change the plan.
          </p>
        ) : available ? (
          <UpgradeButton plan={plan.id} price={plan.price} name={plan.name} />
        ) : (
          <p role="status" className="text-sm text-muted-foreground">
            Checkout for this plan is not configured yet. Contact support for
            availability.
          </p>
        )}
      </CardFooter>
    </Card>
  );
}

function CancelButton({
  planName,
  keepsPlan,
}: {
  planName: string;
  keepsPlan: boolean;
}) {
  const queryClient = useQueryClient();
  const cancel = useMutation({
    mutationFn: cancelSubscription,
    meta: { errorToast: false },
    onSuccess: (status) => {
      queryClient.setQueryData(billingStatusQueryOptions.queryKey, status);
      toast.success(`${planName} is cancelled. You will not be charged again.`);
    },
  });

  return (
    <div className="w-full space-y-3">
      <Button
        variant="outline"
        onClick={() => cancel.mutate()}
        disabled={cancel.isPending}
      >
        {cancel.isPending ? "Cancelling…" : `Cancel ${planName}`}
      </Button>
      <p className="text-sm text-muted-foreground">
        {keepsPlan
          ? `You keep ${planName} until the end of the paid month.`
          : "You will not be charged again."}
      </p>
      {cancel.isError ? (
        <ErrorState
          variant="inline"
          message={getStandardErrorMessage(cancel.error)}
        />
      ) : null}
    </div>
  );
}

function UpgradeButton({
  plan,
  name,
  price,
}: {
  plan: "basic" | "pro" | "agency";
  name: string;
  price: number;
}) {
  const queryClient = useQueryClient();
  const upgrade = useMutation({
    mutationFn: async () => openRazorpayCheckout(await createCheckout(plan)),
    meta: { errorToast: false },
    onSuccess: async (paid) => {
      if (!paid) return;
      toast.success(`Payment received. Welcome to Uplumo ${name}!`);
      await queryClient.invalidateQueries({
        queryKey: billingStatusQueryOptions.queryKey,
      });
    },
  });

  return (
    <div className="w-full space-y-3">
      <Button onClick={() => upgrade.mutate()} disabled={upgrade.isPending}>
        {upgrade.isPending
          ? "Opening checkout…"
          : `Choose ${name} · ${inr.format(price)}/month`}
      </Button>
      {upgrade.isError ? (
        <ErrorState
          variant="inline"
          message={getStandardErrorMessage(upgrade.error)}
        />
      ) : null}
    </div>
  );
}
