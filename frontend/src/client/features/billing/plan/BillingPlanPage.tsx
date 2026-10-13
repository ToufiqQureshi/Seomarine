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
import { getStandardErrorMessage } from "@/client/lib/error-messages";
import { ApiError } from "@/client/lib/uplumoApi";
import {
  billingStatusQueryOptions,
  cancelSubscription,
  createCheckout,
  describeStatus,
  PRO_PRICE_INR,
  type BillingStatus,
} from "./planApi";
import { openRazorpayCheckout } from "./razorpay";

const PRO_FEATURES = [
  "AI traffic analytics: visitors from ChatGPT, Perplexity, Gemini, Claude and Copilot",
  "AI visibility included, no extra add-on",
  "No row limits on your own data",
  "Agency-branded client reports",
];

const inr = new Intl.NumberFormat("en-IN", {
  style: "currency",
  currency: "INR",
  maximumFractionDigits: 0,
});

export function BillingPlanPage() {
  const statusQuery = useQuery(billingStatusQueryOptions);

  return (
    <div className="h-full overflow-auto px-4 py-4 pb-24 md:px-6 md:py-6 md:pb-8">
      <div className="mx-auto max-w-3xl space-y-6">
        <PageHeader
          title="Billing"
          description="Your plan, what it costs and when it renews."
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
          <PlanCard status={statusQuery.data} />
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
  // A retry can't fix an ended session: send the owner to sign in instead.
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

function PlanCard({ status }: { status: BillingStatus }) {
  const isPro = status.plan === "pro";
  const { label, detail, canUpgrade } = describeStatus(status.status);
  const renews = status.currentPeriodEnd
    ? new Date(status.currentPeriodEnd).toLocaleDateString("en-IN", {
        day: "numeric",
        month: "long",
        year: "numeric",
      })
    : null;

  return (
    <Card>
      <CardHeader>
        <CardTitle className="flex items-center gap-2 text-lg">
          {isPro ? "Uplumo Pro" : "Free plan"}
          <Badge variant={status.status === "active" ? "success" : "soft"}>
            {label}
          </Badge>
        </CardTitle>
        <CardDescription>
          {isPro
            ? status.cancelAtPeriodEnd
              ? renews
                ? `Cancelled. Pro stays on until ${renews}, and you will not be charged again.`
                : "Cancelled. You will not be charged again."
              : renews
                ? `Renews on ${renews}.`
                : "Your Pro plan is set up."
            : status.paymentsConfigured
              ? `Upgrade to Pro for ${inr.format(PRO_PRICE_INR)} a month.`
              : "Your current plan. Online payments are temporarily unavailable."}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {detail ? <p className="text-sm">{detail}</p> : null}
        <ul className="space-y-2 text-sm">
          {PRO_FEATURES.map((feature) => (
            <li key={feature} className="flex gap-2">
              <Check className="mt-0.5 size-4 shrink-0 text-primary" />
              {feature}
            </li>
          ))}
        </ul>
      </CardContent>
      {!status.paymentsConfigured ? (
        <CardFooter>
          <p role="status" className="text-sm text-muted-foreground">
            To change or cancel your plan, email{" "}
            <a
              className="underline"
              href="mailto:support@uplumo.com"
            >
              support@uplumo.com
            </a>.
          </p>
        </CardFooter>
      ) : canUpgrade ? (
        isPro ? null : (
          <CardFooter>
            <UpgradeButton />
          </CardFooter>
        )
      ) : status.cancelAtPeriodEnd ? null : (
        // Any subscription that can still charge can be cancelled, including
        // one whose payments are failing.
        <CardFooter>
          <CancelButton keepsPro={status.status === "active"} />
        </CardFooter>
      )}
    </Card>
  );
}

function CancelButton({ keepsPro }: { keepsPro: boolean }) {
  const queryClient = useQueryClient();
  const cancel = useMutation({
    mutationFn: cancelSubscription,
    meta: { errorToast: false },
    onSuccess: (status) => {
      queryClient.setQueryData(billingStatusQueryOptions.queryKey, status);
      toast.success("Pro is cancelled. You will not be charged again.");
    },
  });

  return (
    <div className="w-full space-y-3">
      <Button
        variant="outline"
        onClick={() => cancel.mutate()}
        disabled={cancel.isPending}
      >
        {cancel.isPending ? "Cancelling…" : "Cancel Pro"}
      </Button>
      <p className="text-sm text-muted-foreground">
        {keepsPro
          ? "One click, no forms. You keep Pro until the end of the month you paid for."
          : "One click, no forms. You will not be charged again."}
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

function UpgradeButton() {
  const queryClient = useQueryClient();
  const upgrade = useMutation({
    mutationFn: async () => openRazorpayCheckout(await createCheckout()),
    meta: { errorToast: false },
    onSuccess: async (paid) => {
      if (!paid) return;
      toast.success("Payment received. Welcome to Uplumo Pro!");
      // Razorpay tells our server by webhook, so the plan can lag a moment.
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
          : `Upgrade for ${inr.format(PRO_PRICE_INR)}/month`}
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
