import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link } from "@tanstack/react-router";
import { AlertTriangle, Check } from "lucide-react";
import { useState } from "react";
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
import { SUPPORT_EMAIL } from "@/client/lib/support";
import { ApiError } from "@/client/lib/uplumoApi";
import {
  billingPlansQueryOptions,
  billingStatusQueryOptions,
  cancelSubscription,
  createCheckout,
  describeStatus,
  waitForPaidPlan,
  type BillingPlan,
  type BillingStatus,
} from "./planApi";
import { openRazorpayCheckout } from "./razorpay";

const inr = new Intl.NumberFormat("en-IN", {
  style: "currency",
  currency: "INR",
  maximumFractionDigits: 0,
});

type PaidPlan = BillingPlan & { id: "basic" | "pro" | "agency" };

export function BillingPlanPage() {
  const statusQuery = useQuery(billingStatusQueryOptions);
  const plansQuery = useQuery(billingPlansQueryOptions);
  const canManageBilling = useCanManageBilling();
  const paidPlans = (plansQuery.data ?? []).filter(
    (plan): plan is PaidPlan => plan.id !== "free",
  );

  return (
    <div className="h-full overflow-auto px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <div className="mx-auto max-w-6xl space-y-6">
        <PageHeader
          title="Plan and billing"
          description="Compare what each plan includes and manage your workspace subscription."
        />
        {statusQuery.isPending || plansQuery.isPending ? (
          <SkeletonCard />
        ) : statusQuery.isError ? (
          <StatusError
            error={statusQuery.error}
            onRetry={() => void statusQuery.refetch()}
            isRetrying={statusQuery.isFetching}
          />
        ) : plansQuery.isError ? (
          <StatusError
            error={plansQuery.error}
            onRetry={() => void plansQuery.refetch()}
            isRetrying={plansQuery.isFetching}
          />
        ) : (
          <>
            <PlanLimitWarning status={statusQuery.data} />
            <CurrentPlanCard status={statusQuery.data} plans={paidPlans} />
            <section aria-labelledby="available-plans" className="space-y-4">
              <div>
                <h2 id="available-plans" className="text-xl font-semibold">
                  Compare paid plans
                </h2>
                <p className="text-sm text-muted-foreground">
                  Prices are per workspace, per month. Cancel in one click, any
                  time.
                </p>
              </div>
              <div className="grid gap-4 lg:grid-cols-3">
                {paidPlans.map((plan) => (
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

function PlanLimitWarning({ status }: { status: BillingStatus }) {
  const limits: {
    label: string;
    usage: BillingStatus["usage"]["sites"];
  }[] = [
    { label: "active websites", usage: status.usage.sites },
    { label: "team seats", usage: status.usage.members },
    { label: "tracked keywords", usage: status.usage.trackedKeywords },
  ];
  const exceeded = limits.filter(({ usage }) => usage.exceeded);
  if (exceeded.length === 0) return null;

  return (
    <section
      role="alert"
      className="flex items-start gap-3 rounded-xl border border-warning/40 bg-warning/10 p-4"
    >
      <AlertTriangle className="mt-0.5 size-5 shrink-0" aria-hidden />
      <div>
        <h2 className="font-semibold">Workspace is over its plan limits</h2>
        <p className="mt-1 text-sm text-muted-foreground">
          Your data is safe. Paid data and AI actions are paused until these
          counts fit your plan:
        </p>
        <ul className="mt-2 list-disc pl-5 text-sm">
          {exceeded.map(({ label, usage }) => (
            <li key={label}>
              {usage.used.toLocaleString()} {label}; plan allows{" "}
              {usage.limit.toLocaleString()}
            </li>
          ))}
        </ul>
      </div>
    </section>
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

function CurrentPlanCard({
  status,
  plans,
}: {
  status: BillingStatus;
  plans: BillingPlan[];
}) {
  const planName =
    status.plan === "free"
      ? "Free"
      : (plans.find((plan) => plan.id === status.plan)?.name ?? status.plan);
  const { label, detail, canUpgrade } = describeStatus(status.status);
  const renews = status.currentPeriodEnd
    ? new Date(status.currentPeriodEnd).toLocaleDateString("en-IN", {
        day: "numeric",
        month: "long",
        year: "numeric",
      })
    : null;
  const isPaid = status.plan !== "free" && !status.unlimited;
  const description = status.unlimited
    ? "This is an Uplumo team workspace. Every feature is on, with no limits, and nothing is charged."
    : status.cancelAtPeriodEnd
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
          {status.unlimited ? "Team workspace" : `${planName} plan`}
          <Badge
            variant={
              status.unlimited || status.status === "active"
                ? "success"
                : "soft"
            }
          >
            {status.unlimited ? "Unlimited" : label}
          </Badge>
        </CardTitle>
        <CardDescription>{description}</CardDescription>
      </CardHeader>
      {detail && !status.unlimited ? (
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
  plan: PaidPlan;
  status: BillingStatus;
  canManageBilling: boolean;
}) {
  const isCurrent = status.plan === plan.id && !status.unlimited;
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
        <CardDescription>{plan.summary}</CardDescription>
        <p className="text-2xl font-semibold">
          {inr.format(plan.priceInr)}
          <span className="text-sm font-normal text-muted-foreground">
            /month
          </span>
        </p>
      </CardHeader>
      <CardContent>
        <ul className="space-y-2 text-sm">
          {plan.highlights.map((feature) => (
            <li key={feature} className="flex gap-2">
              <Check className="mt-0.5 size-4 shrink-0 text-primary" />
              <span>{feature}</span>
            </li>
          ))}
        </ul>
      </CardContent>
      <CardFooter className="flex-col items-stretch gap-2">
        {status.unlimited ? (
          <p role="status" className="text-sm text-muted-foreground">
            Already included for this workspace.
          </p>
        ) : isCurrent ? (
          <p role="status" className="text-sm text-muted-foreground">
            This workspace is on this plan.
          </p>
        ) : hasLivePaidSubscription ? (
          <p className="text-sm text-muted-foreground">
            To move to this plan, write to {SUPPORT_EMAIL} from your account and
            we switch you over.
          </p>
        ) : !canManageBilling ? (
          <p className="text-sm text-muted-foreground">
            Only a workspace owner or admin can change the plan.
          </p>
        ) : available ? (
          <UpgradeButton
            plan={plan.id}
            price={plan.priceInr}
            name={plan.name}
          />
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
  const [activating, setActivating] = useState(false);
  const upgrade = useMutation({
    mutationFn: async () => {
      const paid = await openRazorpayCheckout(await createCheckout(plan), name);
      if (!paid) return null;
      setActivating(true);
      try {
        return await waitForPaidPlan();
      } finally {
        setActivating(false);
      }
    },
    meta: { errorToast: false },
    onSuccess: async (status) => {
      if (!status) return;
      if (status.plan === "free") {
        toast.success(
          `Payment received. Uplumo ${name} turns on within a few minutes.`,
        );
      } else {
        toast.success(`Payment received. Welcome to Uplumo ${name}!`);
      }
      queryClient.setQueryData(billingStatusQueryOptions.queryKey, status);
      // Every screen reads the plan and credits; refresh them all.
      await queryClient.invalidateQueries();
    },
  });

  return (
    <div className="w-full space-y-3">
      <Button onClick={() => upgrade.mutate()} disabled={upgrade.isPending}>
        {activating
          ? "Activating your plan…"
          : upgrade.isPending
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
